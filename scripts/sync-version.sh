#!/bin/bash
# 从 package.json 读取版本号并同步到其他文件

set -e

# 获取项目根目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# 从 package.json 读取版本号
VERSION=$(node -p "require('${PROJECT_ROOT}/miaomiaowu/package.json').version")

if [ -z "$VERSION" ]; then
    echo "Error: Failed to read version from package.json"
    exit 1
fi

echo "Syncing version: $VERSION"

# 更新 main.go
sed -i "s/const version = \".*\"/const version = \"$VERSION\"/" "${PROJECT_ROOT}/cmd/server/main.go"
echo "✓ Updated cmd/server/main.go"

# 更新 install.sh
sed -i "s/VERSION=\"v.*\"/VERSION=\"v$VERSION\"/" "${PROJECT_ROOT}/install.sh"
echo "✓ Updated install.sh"

# 更新 quick-install.sh
sed -i "s/VERSION=\"v.*\"/VERSION=\"v$VERSION\"/" "${PROJECT_ROOT}/quick-install.sh"
echo "✓ Updated quick-install.sh"

# 更新 use-version-check.ts
sed -i "s/const CURRENT_VERSION = '.*'/const CURRENT_VERSION = '$VERSION'/" "${PROJECT_ROOT}/miaomiaowu/src/hooks/use-version-check.ts"
echo "✓ Updated miaomiaowu/src/hooks/use-version-check.ts"

echo ""
echo "Version sync completed: $VERSION"
