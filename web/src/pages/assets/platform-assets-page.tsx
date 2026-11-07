import { WorkspacePage, PageHeader } from "@/components/layout/workspace-page";
import { PlatformMaterialLibrary } from "@/components/assets/platform-material-library";
import { PlatformStoragePane } from "@/pages/settings/platform-storage-pane";

export default function PlatformAssetsPage() {
    return (
        <WorkspacePage>
            <PageHeader title="素材库" description="上传、查看和复用 OceanWay 账号中的素材。" />
            <div className="mt-4 rounded-lg border border-border bg-surface p-4">
                <PlatformStoragePane />
            </div>
            <PlatformMaterialLibrary />
        </WorkspacePage>
    );
}
