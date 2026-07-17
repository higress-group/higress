---
name: higress-update-envoy-gateway
description: Update Higress Envoy binary and gateway image dependencies for e2e validation. Use when Codex needs to build Envoy packages from the current Higress branch, upload those packages to higress-group/proxy releases, update Makefile.core.mk ENVOY_PACKAGE_URL_PATTERN and ENVOY_LATEST_IMAGE_TAG, run make build-gateway-local, retag the generated proxy/proxyv2 image as gateway, push the gateway image, or prepare a signed-off PR that lets Higress e2e tests consume a new Envoy build.
---

# Higress Envoy Gateway Dependency Update

## Core Workflow

Run from the Higress repo root unless a step explicitly says otherwise.

1. Verify context:
   - Check `git status --short --branch`.
   - Do not remove unrelated user changes or generated artifacts.
   - Use `gh` for GitHub release/PR checks and always pass `--repo` for non-current repos.

2. Build Envoy packages from the current branch:
   ```bash
   git submodule update --init
   make build-envoy
   ```
   Expected artifacts land in `external/package/`, typically:
   - `envoy-alpha-<proxy-sha>.tar.gz`
   - `envoy-symbol-<proxy-sha>.tar.gz`
   - matching `.sha256` and `.dwp` files

3. Publish Envoy package release in `higress-group/proxy`:
   - Create a new release tag by incrementing the requested RC/test tag, for example `v2.2.4-rc.2-test-cpp-host`.
   - Match the reference release asset names, even if local filenames include SHAs:
     - local `envoy-alpha-<sha>.tar.gz` uploads as `envoy-amd64.tar.gz`
     - local `envoy-symbol-<sha>.tar.gz` uploads as `envoy-symbol-amd64.tar.gz`
   - Use temporary renamed copies rather than renaming source artifacts:
     ```bash
     cp external/package/envoy-alpha-<sha>.tar.gz /tmp/envoy-amd64.tar.gz
     cp external/package/envoy-symbol-<sha>.tar.gz /tmp/envoy-symbol-amd64.tar.gz
     gh release create <release-tag> /tmp/envoy-amd64.tar.gz /tmp/envoy-symbol-amd64.tar.gz \
       --repo higress-group/proxy --target <target-branch> --title <release-tag> --generate-notes
     gh release view <release-tag> --repo higress-group/proxy --json tagName,targetCommitish,assets,url
     ```
   - If following an existing reference release, inspect it first with `gh release view`.

4. Update Higress Makefile dependencies:
   - In `Makefile.core.mk`, set:
     ```make
     export ENVOY_PACKAGE_URL_PATTERN?=https://github.com/higress-group/proxy/releases/download/<release-tag>/envoy-symbol-ARCH.tar.gz
     ```
   - After building and pushing the gateway image, set:
     ```make
     ENVOY_LATEST_IMAGE_TAG ?= <gateway-image-tag>
     ```

5. Build the local gateway image:
   ```bash
   make build-gateway-local
   ```
   Watch the log to confirm Envoy downloads from the new release URL. The build target may emit an image under a proxy-style repository/name, commonly:
   - `higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/proxyv2:<tag>`

6. Retag proxy image as gateway:
   ```bash
   docker tag higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/proxyv2:<tag> \
     higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway:<tag>
   docker images --format '{{.Repository}}:{{.Tag}} {{.ID}} {{.CreatedSince}}' \
     higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway
   ```
   Verify the `gateway:<tag>` and `proxyv2:<tag>` image IDs match.

7. Push the gateway image when requested:
   ```bash
   docker push higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway:<tag>
   ```
   Record the pushed digest in the final response.

8. Commit and push Makefile changes:
   - Commit only intended tracked files. Do not add `plugins/golang-filter/golang-filter_amd64.so` or other build outputs unless explicitly requested.
   - Use DCO sign-off:
     ```bash
     git add Makefile.core.mk
     git commit -s -m "Update gateway envoy dependencies"
     git push origin <branch>
     ```
   - If DCO fails after a previous unsigned commit:
     ```bash
     git commit --amend --no-edit --signoff
     git push --force-with-lease origin <branch>
     gh pr checks <pr-number> --repo higress-group/higress
     ```

## Practical Notes

- `make build-gateway-local` may need Docker daemon access, network access, and write access to repo/submodule state.
- If sandboxed commands fail with read-only filesystem errors, Docker socket permission errors, or network failures, rerun the same important command with elevated permissions and a concrete justification.
- The default local gateway tag usually comes from the current Git revision shown in the build log as `TAG=<sha>`.
- For e2e validation, the point of this workflow is to make `install-dev`, `install-dev-wasmplugin`, and local image update targets use the newly pushed gateway image through `ENVOY_LATEST_IMAGE_TAG`.
