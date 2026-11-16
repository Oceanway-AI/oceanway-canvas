"""Recompute Studio W7 admission and first-frame percentiles from the original samples."""

import json
import math
from datetime import datetime
from pathlib import Path
import sys


def check(condition, message):
    if not condition:
        raise ValueError(message)


def percentiles(values):
    ordered = sorted(value for value in values if value is not None)
    observed = len(ordered)
    ordered.extend([None] * (len(values) - observed))
    return {
        "group_size": len(values),
        "observed": observed,
        "missing_or_failed": len(values) - observed,
        "p50_ms": ordered[math.ceil(len(values) * 0.50) - 1],
        "p95_ms": ordered[math.ceil(len(values) * 0.95) - 1],
    }


def utc_ms(value):
    return datetime.fromisoformat(value.replace("Z", "+00:00")).timestamp() * 1000


def verify(path):
    receipt = json.loads(path.read_text())
    samples = receipt["samples"]
    check(len(samples) == receipt["requested"] == 30, "fixed sample count")
    check([row["ordinal"] for row in samples] == list(range(1, 31)), "fixed ordinal order")
    check(len({row["original_request"] for row in samples}) == 30, "distinct original requests")
    times = {"click_platform": [], "click_browser": []}
    if receipt["family"] == "video":
        times["publication_frame"] = []
    files = {}
    pinned = 0
    one_attempt = 0
    zero_settled = 0
    status_reads = 0
    exact_traces = set()
    for row in samples:
        original = row["original_request"]
        check(row["original_posts"] == 1, f"{original} submitted once")
        writes = row["original_indexeddb"]
        check(len(writes) == 1 and writes[0]["command"] == original, f"{original} IndexedDB")
        check(writes[0]["putMs"] <= writes[0]["committedMs"] <= row["browser_send_ms"], f"{original} commit before send")
        accepted = row["platform"] and row["platform"]["http"] in (200, 202)
        click = row["trusted_click_event_ms"]
        admission = round(row["platform"]["ended_ms"] - click, 3) if accepted else None
        check(admission == row["click_to_platform_ms"], f"{original} durable receipt")
        times["click_platform"].append(admission)
        times["click_browser"].append(row["browser_receipt_ms"] - click if accepted else None)
        one_attempt += int(row["source_attempts"] == len(row["source_attempt_ids"]) == 1)
        funds = row.get("funds") or {}
        zero_settled += int(funds.get("funds_state") == "settled" and funds.get("reserved_points") == funds.get("settled_points") == "0")
        if clocks := row.get("exact_request_clocks"):
            provider = clocks["platform_post_by_operation"]
            sdk = clocks["sdk"]
            identity = clocks["studio_post_identity"]
            check(sdk["task"] == row["studio_task"] and sdk["trace"] == identity["trace"], f"{original} SDK request")
            check(identity["provider_trace"] == clocks["platform_identity"]["trace"], f"{original} resolved identity")
            check(provider["trace"] in clocks["original_operation_invoke_traces"], f"{original} ingress trace")
            check(provider["ended_utc"] == row["platform"]["ended_utc"], f"{original} timed admission")
            check(sdk["start_us"] <= sdk["write_us"] <= sdk["first_byte_us"] <= sdk["returned_us"], f"{original} transport order")
            check(all(e["path"] == "/internal/products/v1/media-generations/" + original for e in clocks["status_gets"]), f"{original} GET scope")
            exact_traces.add(provider["trace"])
            status_reads += len(clocks["status_gets"])
        if "publication_frame" in times:
            frame = row["initial_frame"]
            duration = None
            if frame:
                resources = {r["resource_id"] for r in row["studio_material_references"]}
                matches_message = frame.get("messageCommand") == original and frame.get("messageID") == writes[0].get("messageID")
                matches_output = frame.get("hostTask") == row["studio_task"] or frame.get("resource") in resources
                check(matches_message or matches_output, f"{original} frame belongs to original")
                check(frame["width"] > 0 and frame["height"] > 0 and frame["presentedFrames"] > 0, f"{original} presented frame")
                if viewport := frame.get("viewport"):
                    check(viewport["css"] and viewport["width"] > 0 and viewport["height"] > 0, f"{original} clipped viewport")
                duration = round(frame["timeMs"] - utc_ms(row["gateway_published_at"]), 3)
            check(duration == row["publication_to_frame_ms"], f"{original} publication to frame")
            times["publication_frame"].append(duration)
        matched = False
        for archive in row["material_archive"]:
            if not archive.get("file_id"):
                continue
            files[archive["file_id"]] = archive["charged_original_bytes"]
            check(archive["recorded_sha256"] == archive["stored_sha256"], f"{original} archive digest")
            for resource in row["studio_material_references"]:
                same_file = all(resource[key] == archive[key] for key in ("asset_id", "version_id", "file_id"))
                if same_file and resource["sha256"] == archive["stored_sha256"]:
                    matched = True
        pinned += int(matched)
        if restored := row.get("close_reopen"):
            check(restored["sameOriginal"] and restored["newCommands"] == restored["newAdoptions"] == 0, f"{original} restored original")
    check(len(files) == receipt["counts"]["immutable_archived_files"], "distinct archived files")
    check(sum(files.values()) == receipt["counts"]["charged_original_bytes"], "stored original byte count")
    check(pinned == receipt["counts"]["studio_resources_pinned_to_platform_files"], "Studio resources match Platform files")
    check(one_attempt == receipt["counts"]["one_supplier_attempt"], "one supplier attempt per original")
    check(zero_settled == receipt["counts"]["reserved_and_settled_zero"], "original zero settlement count")
    if correlations := receipt.get("clock_correlation"):
        check(len(exact_traces) == correlations["observed"]["operation_trace_matches"], "unique ingress traces")
        check(status_reads == correlations["observed"]["status_gets"], "original status GET count")
    for key, values in times.items():
        expected = receipt["summaries"][key]
        for field, value in percentiles(values).items():
            check(expected[field] == value, f"{key} {field}, expected {expected[field]}, recalculated {value}")
    return {key: percentiles(values) for key, values in times.items()}


if __name__ == "__main__":
    paths = [Path(arg) for arg in sys.argv[1:]] or sorted(Path(__file__).parent.glob("w7-*-fixed-verified.json"))
    for path in paths:
        print(json.dumps({"file": path.name, "measurements": verify(path)}))
