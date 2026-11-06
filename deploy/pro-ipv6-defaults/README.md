# B-side defaults for new Pro accounts

This deployment-only policy gives new OpenAI OAuth Pro accounts five fixed IPv6
exits when no proxy selection was supplied. It runs on database INSERT, covering
manual account creation, imports and API pushes. Existing accounts, explicit
primary/fallback choices, credential shadows and non-Pro accounts are preserved.
The existing single-exit policy continues handling unassigned non-Pro accounts.

Prerequisites: the existing `b_oauth_ipv6_policy` secret, a healthy loopback-only
CONNECT adapter on port 26448, and its persistent local IPv6 /80 return route.
The adapter uses the existing signed `b-account-N` protocol with a separate /80
base. Virtual ID `account_id * 8 + slot`, slots 1 through 5, gives distinct
addresses without collisions with existing accounts or other adapter ranges.
Credentials are computed server-side and never committed.

Install automatic-five.sql transactionally only after adapter egress checks and
a rolled-back trigger test. The extra four IDs are stored in the existing
`extra.proxy_fallback_ids` field and use proxy type `pro`. The default applies at
creation; upgrading an existing account to Pro does not rewrite its routes.

Disable future automatic assignment by dropping only
`b_oauth_00_new_pro_ipv6_five`. Keep the adapter running for accounts already
assigned to it. Reverting the rule does not migrate or disconnect those accounts.
