# Visible state rules editor: 0.2.6.18

The Codex state page exposes an expanded manual rules editor with subscription
type, upstream model and ordered target-length fields. Rules can be added,
edited and removed without searching for the upper-right settings button.
Saving preserves unrelated acquisition and proxy settings.

Pro and Team defaults both return to `[332,292]`, in that order, for all models.
The previous Team 286/273 defaults are removed. Custom per-model rules remain
available and state values remain isolated by account and model.

The production settings were restored through the authenticated admin API
before this release. Deployment must preserve saved settings, port 6064 and
active streams. Retain previous hashed frontend assets and stop temporary
containers only after their TCP connections drain.
