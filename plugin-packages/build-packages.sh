#!/bin/sh
set -eu

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
node "$root_dir/embed-documentation.mjs"

package_plugin() {
  package_id=$1
  package_dir="$root_dir/$package_id"
  output_file="$root_dir/$package_id.yingce-plugin"
  temporary_file="$root_dir/.$package_id.yingce-plugin.tmp"
  rm -f "$temporary_file"
  (
    cd "$package_dir"
    find manifest.json README.md docs assets web backend LICENSE -type f 2>/dev/null | LC_ALL=C sort | zip -X -q "$temporary_file" -@
  )
  mv "$temporary_file" "$output_file"
}

package_ids=$(node "$root_dir/package-directories.mjs")
for package_id in $package_ids; do
  package_plugin "$package_id"
done
