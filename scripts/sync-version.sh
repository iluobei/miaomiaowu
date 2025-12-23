#!/bin/bash
# 从 package.json 读取版本号并同步到其他文件

set -e

# 获取项目根目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# 从 package.json 读取版本号
VERSION=$(node -p "require('${PROJECT_ROOT}/miaomiaowu/package.json').version")

if [ -z "$VERSION" ]; then
    echo "版本号读取失败: Failed to read version from package.json"
    exit 1
fi

echo "更新版本号: $VERSION"

# 更新 internal/version/version.go
sed -i "s/const version = \".*\"/const version = \"$VERSION\"/" "${PROJECT_ROOT}/internal/version/version.go"
echo "✓ 更新成功 internal/version/version.go"

# 更新 install.sh
sed -i "s/VERSION=\"v.*\"/VERSION=\"v$VERSION\"/" "${PROJECT_ROOT}/install.sh"
echo "✓ 更新成功 install.sh"

# 更新 quick-install.sh
sed -i "s/VERSION=\"v.*\"/VERSION=\"v$VERSION\"/" "${PROJECT_ROOT}/quick-install.sh"
echo "✓ 更新成功 quick-install.sh"

# 更新 use-version-check.ts
sed -i "s/const CURRENT_VERSION = '.*'/const CURRENT_VERSION = '$VERSION'/" "${PROJECT_ROOT}/miaomiaowu/src/hooks/use-version-check.ts"
echo "✓ 更新成功 miaomiaowu/src/hooks/use-version-check.ts"

echo ""
echo "版本号同步完成: $VERSION"
