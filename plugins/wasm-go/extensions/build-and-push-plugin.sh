#!/bin/bash

# Higress 插件构建和推送脚本
# 用途：构建 WASM 插件，打包成 Docker 镜像，并推送到华为云 SWR

set -e  # 遇到错误时退出

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印带颜色的消息
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查参数
if [ $# -lt 1 ]; then
    echo "用法: $0 <插件名> [插件目录路径] [Dockerfile路径]"
    echo ""
    echo "示例:"
    echo "  # 从插件目录内执行"
    echo "  $0 consumer-group-mapping"
    echo ""
    echo "  # 从 extensions 目录执行"
    echo "  $0 consumer-group-mapping ./consumer-group-mapping"
    echo ""
    echo "  # 指定完整的插件目录路径"
    echo "  $0 consumer-group-mapping /path/to/consumer-group-mapping"
    echo ""
    echo "  # 使用自定义 Dockerfile"
    echo "  $0 consumer-group-mapping ./consumer-group-mapping ./consumer-group-mapping/Dockerfile.custom"
    exit 1
fi

PLUGIN_NAME=$1
PLUGIN_DIR=${2:-"./${PLUGIN_NAME}"}
DOCKERFILE_PATH=${3:-"${PLUGIN_DIR}/Dockerfile"}

# 如果插件目录是相对路径且不存在，尝试在当前目录的子目录中查找
if [ ! -d "${PLUGIN_DIR}" ]; then
    print_info "插件目录 ${PLUGIN_DIR} 不存在，尝试在当前目录查找..."
    if [ -d "${PWD}/${PLUGIN_NAME}" ]; then
        PLUGIN_DIR="${PWD}/${PLUGIN_NAME}"
        print_success "找到插件目录: ${PLUGIN_DIR}"
    else
        print_error "无法找到插件目录: ${PLUGIN_NAME}"
        print_info "请确认插件目录存在或提供正确的路径"
        exit 1
    fi
fi

# 获取插件目录的绝对路径
PLUGIN_DIR=$(cd "${PLUGIN_DIR}" && pwd)

# 设置 Dockerfile 路径
if [ "${DOCKERFILE_PATH}" = "${PLUGIN_DIR}/Dockerfile" ] && [ ! -f "${DOCKERFILE_PATH}" ]; then
    # 尝试在插件目录中查找 Dockerfile
    if [ -f "${PLUGIN_DIR}/Dockerfile" ]; then
        DOCKERFILE_PATH="${PLUGIN_DIR}/Dockerfile"
    fi
fi

# 生成时间戳版本号
TIMESTAMP=$(date +"%Y%m%d%H%M%S")
IMAGE_NAME="${PLUGIN_NAME}"
SWR_REGISTRY="swr.cn-east-3.myhuaweicloud.com"
SWR_NAMESPACE="tob"
SWR_IMAGE_NAME="higress-${PLUGIN_NAME}"

# ============ git 护栏（OPE-9466，09-21 归因断链事故跟进） ============
# 事故背景：本脚本编译输入是执行者本机工作副本。09-20 在落后于 wujieai-master
# 的 checkout 上构建 ai-token-report，产出丢失 wujie_task_id 透传的镜像上生产，
# 托管计费归因断链 ~17h。以下护栏确保：镜像可溯源（tag 带 commit）、
# 不在脏 checkout / 落后远端的 checkout 上构建。
if [ "${SKIP_GIT_GUARD}" = "1" ]; then
    print_warning "SKIP_GIT_GUARD=1：已跳过 git 护栏（紧急逃生口，请勿常态化使用）"
fi

REPO_ROOT=$(git -C "${PLUGIN_DIR}" rev-parse --show-toplevel 2>/dev/null)
if [ -z "${REPO_ROOT}" ]; then
    print_error "插件目录不在 git 仓库内，拒绝构建：镜像将无法追溯到源码版本（如确需跳过，设 SKIP_GIT_GUARD=1）"
    [ "${SKIP_GIT_GUARD}" = "1" ] || exit 1
fi

GIT_BRANCH=$(git -C "${REPO_ROOT}" rev-parse --abbrev-ref HEAD 2>/dev/null)
COMMIT_ID=$(git -C "${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null)
IMAGE_TAG="${TIMESTAMP}-${COMMIT_ID:-unknown}"

if [ "${SKIP_GIT_GUARD}" != "1" ]; then
    # 护栏 1：拒绝脏 checkout（已跟踪文件有未提交改动时，无法确定镜像对应哪份源码）
    if [ -n "$(git -C "${REPO_ROOT}" status --porcelain --untracked-files=no 2>/dev/null)" ]; then
        print_error "工作副本有未提交改动，拒绝构建：请先 commit 或 stash（否则镜像无法对应确定的源码版本）"
        exit 1
    fi

    if [ "${GIT_BRANCH}" = "HEAD" ]; then
        print_warning "当前处于 detached HEAD（${COMMIT_ID}），请确认这是你有意构建的版本"
    fi

    # 护栏 2：拒绝落后远端的 checkout（09-20 事故的直接机制）
    if [ "${GIT_BRANCH}" != "HEAD" ] && git -C "${REPO_ROOT}" fetch origin "${GIT_BRANCH}" >/dev/null 2>&1 \
        && git -C "${REPO_ROOT}" rev-parse --verify -q "origin/${GIT_BRANCH}" >/dev/null; then
        BEHIND=$(git -C "${REPO_ROOT}" rev-list --count HEAD..origin/"${GIT_BRANCH}")
        if [ "${BEHIND}" -gt 0 ]; then
            print_error "本地落后 origin/${GIT_BRANCH} ${BEHIND} 个提交，拒绝构建：请先 git pull --ff-only"
            print_error "（09-21 归因断链事故即由在旧 checkout 上构建导致，详见 OPE-9437）"
            exit 1
        fi
    else
        print_warning "无法与 origin/${GIT_BRANCH} 比对（分支可能尚未推送或无网络），跳过落后检查——tag 仍带 commit ${COMMIT_ID} 可溯源"
    fi
fi
# ============ git 护栏结束 ============

print_info "======================================"
print_info "Higress 插件构建和推送脚本"
print_info "======================================"
print_info "插件名称: ${PLUGIN_NAME}"
print_info "插件目录: ${PLUGIN_DIR}"
print_info "源码版本: ${BRANCH}@${COMMIT_ID:-unknown}"
print_info "版本号: ${IMAGE_TAG}"
print_info "Dockerfile: ${DOCKERFILE_PATH}"
print_info "SWR 仓库: ${SWR_REGISTRY}/${SWR_NAMESPACE}/${SWR_IMAGE_NAME}:${IMAGE_TAG}"
print_info "======================================"

# 切换到插件目录
ORIGINAL_DIR=$(pwd)
cd "${PLUGIN_DIR}"

# 1. 清理旧的 WASM 文件
print_info "步骤 1: 清理旧的 WASM 文件..."
if [ -f "main.wasm" ]; then
    rm -f main.wasm
    print_success "已删除旧的 main.wasm"
else
    print_info "没有找到旧的 main.wasm 文件"
fi

# 2. 检查构建脚本
print_info "步骤 2: 检查构建脚本..."
if [ -f "./build.sh" ]; then
    BUILD_SCRIPT="./build.sh"
    print_success "找到构建脚本: ${BUILD_SCRIPT}"

    # 3. 执行构建脚本
    print_info "步骤 3: 执行构建脚本..."
    if chmod +x ./build.sh; then
        ./build.sh
        if [ $? -eq 0 ]; then
            print_success "构建脚本执行成功"
        else
            print_error "构建脚本执行失败"
            exit 1
        fi
    else
        print_error "无法执行构建脚本"
        exit 1
    fi

    # 4. 检查 WASM 文件是否生成
    print_info "步骤 4: 检查 WASM 文件..."
    if [ ! -f "main.wasm" ]; then
        print_error "构建完成后未找到 main.wasm 文件"
        exit 1
    fi
    print_success "main.wasm 文件已生成"
else
    print_warning "未找到 build.sh 脚本"
    print_info "检查是否已存在 main.wasm 文件..."

    # 如果没有 build.sh，检查是否已经有 main.wasm
    if [ -f "main.wasm" ]; then
        print_success "找到现有的 main.wasm 文件，跳过构建步骤"
    else
        print_error "既没有 build.sh 脚本，也没有 main.wasm 文件"
        print_info "请确保插件目录中包含 build.sh 或已编译的 main.wasm"
        exit 1
    fi
fi

# 5. 检查 Dockerfile
print_info "步骤 5: 检查 Dockerfile..."
if [ ! -f "${DOCKERFILE_PATH}" ]; then
    # 尝试在插件目录中查找 Dockerfile
    if [ -f "./Dockerfile" ]; then
        DOCKERFILE_PATH="./Dockerfile"
        print_success "在插件目录中找到 Dockerfile: ${DOCKERFILE_PATH}"
    else
        print_error "未找到 Dockerfile: ${DOCKERFILE_PATH}"
        print_info "请确保 Dockerfile 存在于插件目录中"
        exit 1
    fi
fi

# 6. 构建 Docker 镜像
print_info "步骤 6: 构建 Docker 镜像..."
docker build -t "${IMAGE_NAME}:${IMAGE_TAG}" -f "${DOCKERFILE_PATH}" .
if [ $? -eq 0 ]; then
    print_success "Docker 镜像构建成功: ${IMAGE_NAME}:${IMAGE_TAG}"
else
    print_error "Docker 镜像构建失败"
    exit 1
fi

# 7. 标记镜像为 SWR 格式
print_info "步骤 7: 标记镜像为 SWR 格式..."
FULL_SWR_IMAGE="${SWR_REGISTRY}/${SWR_NAMESPACE}/${SWR_IMAGE_NAME}:${IMAGE_TAG}"
docker tag "${IMAGE_NAME}:${IMAGE_TAG}" "${FULL_SWR_IMAGE}"
if [ $? -eq 0 ]; then
    print_success "镜像标记成功: ${FULL_SWR_IMAGE}"
else
    print_error "镜像标记失败"
    exit 1
fi

# 8. 推送到 SWR
print_info "步骤 8: 推送镜像到华为云 SWR..."
print_info "这可能需要几分钟，请耐心等待..."
docker push "${FULL_SWR_IMAGE}"
if [ $? -eq 0 ]; then
    print_success "镜像推送成功!"
else
    print_error "镜像推送失败"
    exit 1
fi

# 9. 显示结果摘要
print_info "======================================"
print_success "构建和推送完成！"
print_info "======================================"
print_info "本地镜像: ${IMAGE_NAME}:${IMAGE_TAG}"
print_info "SWR 镜像: ${FULL_SWR_IMAGE}"
print_info ""
print_info "使用方法:"
print_info "  在 Higress 中配置插件时使用: ${FULL_SWR_IMAGE}"
print_info "======================================"

# 10. 清理本地镜像（可选）
print_warning "是否要删除本地构建的镜像? (y/N)"
read -r response
if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
    print_info "删除本地镜像..."
    docker rmi "${IMAGE_NAME}:${IMAGE_TAG}" "${FULL_SWR_IMAGE}"
    print_success "本地镜像已删除"
fi

# 切换回原始目录
cd "${ORIGINAL_DIR}"

print_success "脚本执行完成!"
