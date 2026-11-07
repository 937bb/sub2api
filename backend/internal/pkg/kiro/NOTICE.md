# Kiro protocol attribution

Protocol reference: hank9999/kiro.rs, MIT License, commit
8fb3dd691d1cedf437bd6a4e9ef9f60b3f217fdd.
https://github.com/hank9999/kiro.rs

This Go adapter follows the credential, conversation and AWS event-stream
structures of that implementation. Account scheduling, persistence, cache
isolation and accounting belong to sub2api. No simulated cache usage is used.

Primary reference for nullable imports, credential refresh diagnostics and
profile-derived API region selection: ZyphrZero/kiro.rs, MIT License, commit
3e20b44af28e1f45ec8a74241236443ede905418.
https://github.com/ZyphrZero/kiro.rs

The runtime-v3 generation endpoint remains the verified default. Explicit CLI
transport selection follows the same reference endpoint/cli.rs envelope and
headers. Simulated prefix-cache accounting is not adopted.
