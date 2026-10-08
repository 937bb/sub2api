# 937sub2b deployment identity

The production B service uses `git@github.com:937bb/sub2api.git`, branch
`937sub2b`, from `/www/wwwroot/sub2api-b-v290-merge`.
`937bb/937sub2api` and `/www/wwwroot/937sub2api` are a different development
line. Do not deploy them into `sub2api-b`, including when adding account import
labels. A Docker tag containing `v2.9.11` does not prove the binary version.

Build a clean, reviewed commit with:

```sh
sh deploy/build-937sub2b.sh FULL_EXPECTED_COMMIT
```

The script checks the source repository and release baseline, injects the
version and commit, adds OCI metadata, and runs the immutable image's version
command before reporting success. Deploy that verified image ID. Assert both
the container ID and image ID before changing traffic or draining an instance;
abort if another deployment has replaced either instance.

Use the shared `/run/sub2api-b-release.lock` with `flock` for deployment
operations. Preserve the existing PostgreSQL, Redis and `/app/data` settings.
C must continue calling `127.0.0.1:6064`; change its proxy target only after the
candidate passes internal C-side business requests. Update the two public
static roots as well as the backend, preserving old hashed assets for active
browser sessions. Keep the previous container and immutable image for rollback.

The SIWC account badge accepts both a verified grant's
`credentials.auth_mode=siwc` and the rotation import marker
`extra.auth_protocol=siwc`. The latter is display metadata only. Only the actual
credential grant selects the public Responses endpoint; imported legacy Codex
OAuth tokens continue using the Codex endpoint.
