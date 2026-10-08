#!/bin/sh
# Build only the canonical B-side repository and verify the actual binary identity.
set -eu
release_commit=${1:?Usage: sh deploy/build-937sub2b.sh EXPECTED_COMMIT}
repo=$(git rev-parse --show-toplevel)
cd "$repo"
case "$(git config --get remote.origin.url)" in
  git@github.com:937bb/sub2api.git|https://github.com/937bb/sub2api.git) ;;
  *) echo 'Refusing B-side build: origin must be 937bb/sub2api, not 937bb/937sub2api' >&2; exit 1 ;;
esac
test "$(git rev-parse HEAD)" = "$release_commit"
test -z "$(git status --porcelain)"
release_version=$(tr -d '\r\n' < backend/cmd/server/VERSION)
case "$release_version" in
  2.*) ;;
  *) echo 'Refusing B-side build: expected the maintained 2.x release baseline' >&2; exit 1 ;;
esac
release_short=$(git rev-parse --short=9 HEAD)
release_name="v${release_version}-937sub2b-siwc"
release_image="sub2api-b:${release_name}-${release_short}"
DOCKER_BUILDKIT=1 docker build \
  --build-arg "VERSION=$release_name" \
  --build-arg "COMMIT=$release_short" \
  --label "org.opencontainers.image.revision=$release_commit" \
  --label "org.opencontainers.image.version=$release_name" \
  --label 'org.opencontainers.image.source=https://github.com/937bb/sub2api' \
  -t "$release_image" .
release_id=$(docker image inspect "$release_image" --format '{{.Id}}')
release_banner=$(docker run --rm --network none --entrypoint /app/sub2api "$release_id" --version 2>&1)
printf '%s\n' "$release_banner" | grep -F "Sub2API $release_name (commit: $release_short,"
printf 'Verified image: %s\nImmutable image ID: %s\n' "$release_image" "$release_id"
