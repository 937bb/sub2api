# Revision 8 interaction-surface coverage

This is a source coverage ledger, not a claim that every conditional state has been exercised in a browser. The inventory contains 111 files, 121 BaseDialog uses (including dynamic components), 42 ConfirmDialog uses and 26 Teleport occurrences. Counts overlap by composition. No backend contract, billing behavior or C deployment changes are included.

## Representative rendered and interaction evidence

Desktop and 390px mobile captures cover user creation, account editing, request billing details and operations settings. Additional evidence covers user edit, group editor, SIWC, redemption, nested OTP, balance, announcement reading, payment/QR, dark confirmation and dark image preview. New API reference source and before/after evidence are named in the incumbent and finish reviews.

Keyboard verification covers nested modal autofocus, Tab/Shift-Tab wrapping, cancellation restoring parent focus, body-lock retention and selector portals. Automated tests cover shared image preview and existing payment, OTP and announcement behavior. This does not certify every provider-specific account branch, real payment settlement, software keyboards on physical phones, every locale or every conditional error state. Preview data is synthetic.

## Per-file disposition

Direct means the caller or its interior was changed. Shared means its existing primitive inherits the new global surface system without changing its business handlers. Anchored portals retain their positioning/interaction behavior and join the shared popover or tooltip styling; image viewers use ImagePreview. A portal occurrence can be an empty legacy wrapper around a migrated primitive.

| File (under frontend/src/) | Dialog / confirmation / portal | Disposition |
| --- | --- | --- |
| components/account/AccountGroupsCell.vue | 0 / 0 / 1 | Direct + shared surface · portal treatment |
| components/account/AccountStatsModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/account/AccountTestModal.vue | 1 / 0 / 1 | Direct + shared surface · portal treatment |
| components/account/BulkEditAccountModal.vue | 1 / 1 / 0 | Direct + shared surface |
| components/account/ClaudeResetCreditsCell.vue | 0 / 1 / 0 | Shared primitive inheritance |
| components/account/CreateAccountModal.vue | 2 / 1 / 0 | Direct + shared surface |
| components/account/EditAccountModal.vue | 1 / 1 / 0 | Direct + shared surface |
| components/account/OllamaCloudUsageSettings.vue | 0 / 1 / 0 | Shared primitive inheritance |
| components/account/OpenAIQuotaResetCell.vue | 0 / 1 / 0 | Shared primitive inheritance |
| components/account/OpenAIReferralCell.vue | 1 / 0 / 0 | Direct + shared surface |
| components/account/ReAuthAccountModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/account/SIWCAccountModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/account/SyncFromCrsModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/account/TempUnschedStatusModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/AdminComplianceDialog.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/ErrorPassthroughRulesModal.vue | 2 / 1 / 0 | Direct + shared surface |
| components/admin/TLSFingerprintProfilesModal.vue | 2 / 1 / 0 | Direct + shared surface |
| components/admin/account/AccountActionMenu.vue | 0 / 0 / 1 | Direct + shared surface · portal treatment |
| components/admin/account/AccountStatsModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/account/AccountTestModal.vue | 1 / 0 / 1 | Direct + shared surface · portal treatment |
| components/admin/account/ImportDataModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/account/ReAuthAccountModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/account/ScheduledTestsPanel.vue | 1 / 1 / 0 | Direct + shared surface |
| components/admin/announcements/AnnouncementReadStatusDialog.vue | 1 / 0 / 0 | Shared primitive inheritance |
| components/admin/group/GroupRPMOverridesModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/group/GroupRateMultipliersModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/monitor/MonitorFormDialog.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/monitor/MonitorKeyPickerDialog.vue | 1 / 0 / 0 | Shared primitive inheritance |
| components/admin/monitor/MonitorRunResultDialog.vue | 1 / 0 / 0 | Shared primitive inheritance |
| components/admin/monitor/MonitorTemplateApplyPickerDialog.vue | 1 / 0 / 0 | Shared primitive inheritance |
| components/admin/monitor/MonitorTemplateManagerDialog.vue | 1 / 1 / 0 | Direct + shared surface |
| components/admin/payment/AdminOrderDetail.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/payment/AdminRefundDialog.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/proxy/ImportDataModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/subscription/BulkSubscriptionActionDialog.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/usage/UsageCleanupDialog.vue | 1 / 2 / 0 | Direct + shared surface |
| components/admin/usage/UsageDetailDialog.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/usage/UsageTable.vue | 0 / 0 / 2 | Shared primitive inheritance · portal treatment |
| components/admin/user/BulkEditUserModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/user/GroupReplaceModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/user/UserAllowedGroupsModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/user/UserApiKeysModal.vue | 1 / 0 / 1 | Direct + shared surface · portal treatment |
| components/admin/user/UserBalanceHistoryModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/user/UserBalanceModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/user/UserCreateModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/user/UserEditModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/admin/user/UserPlatformQuotaModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/auth/LoginAgreementPrompt.vue | 1 / 0 / 1 | Direct + shared surface · portal treatment |
| components/auth/TotpLoginModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/auth/TotpStepUpDialog.vue | 1 / 0 / 0 | Direct + shared surface |
| components/channels/SupportedModelChip.vue | 0 / 0 / 1 | Direct + shared surface · portal treatment |
| components/common/AnnouncementBell.vue | 2 / 0 / 0 | Direct + shared surface |
| components/common/AnnouncementPopup.vue | 1 / 0 / 0 | Direct + shared surface |
| components/common/BaseDialog.vue | 0 / 0 / 1 | Direct + shared surface · portal treatment |
| components/common/ConfirmDialog.vue | 1 / 0 / 0 | Direct + shared surface |
| components/common/ExportProgressDialog.vue | 1 / 0 / 0 | Direct + shared surface |
| components/common/HelpTooltip.vue | 0 / 0 / 1 | Direct + shared surface · portal treatment |
| components/common/ImagePreview.vue | 1 / 0 / 0 | Direct + shared surface |
| components/common/Select.vue | 0 / 0 / 1 | Direct + shared surface · portal treatment |
| components/common/Toast.vue | 0 / 0 / 1 | Direct + shared surface · portal treatment |
| components/common/VersionBadge.vue | 0 / 0 / 1 | Direct + shared surface · portal treatment |
| components/keys/BulkEditKeysModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/keys/UseKeyModal.vue | 1 / 0 / 0 | Direct + shared surface |
| components/payment/PaymentProviderDialog.vue | 1 / 0 / 0 | Direct + shared surface |
| components/payment/PaymentQRDialog.vue | 1 / 0 / 0 | Direct + shared surface |
| components/user/MonitorDetailDialog.vue | 1 / 0 / 0 | Shared primitive inheritance |
| components/user/UserAttributesConfigModal.vue | 2 / 1 / 0 | Direct + shared surface |
| components/user/UserErrorDetailModal.vue | 1 / 0 / 0 | Shared primitive inheritance |
| components/user/profile/ProfilePasskeyCard.vue | 1 / 0 / 0 | Direct + shared surface |
| components/user/profile/TotpDisableDialog.vue | 1 / 0 / 0 | Direct + shared surface |
| components/user/profile/TotpSetupModal.vue | 1 / 0 / 0 | Direct + shared surface |
| features/channel-monitor-v2/FilterMultiSelect.vue | 0 / 0 / 1 | Direct + shared surface · portal treatment |
| features/channel-monitor-v2/RelayPulseMatrix.vue | 0 / 0 / 1 | Direct + shared surface · portal treatment |
| features/prompt-audit/PromptAuditView.vue | 0 / 2 / 0 | Shared primitive inheritance |
| features/prompt-audit/components/EndpointPool.vue | 1 / 0 / 0 | Direct + shared surface |
| features/prompt-audit/components/EventDetailDialog.vue | 1 / 0 / 0 | Direct + shared surface |
| features/prompt-audit/components/FilterDeleteDialog.vue | 1 / 0 / 0 | Direct + shared surface |
| views/admin/AccountsView.vue | 0 / 3 / 1 | Direct + shared surface · portal treatment |
| views/admin/AnnouncementsView.vue | 1 / 1 / 0 | Direct + shared surface |
| views/admin/AuditLogView.vue | 3 / 1 / 0 | Direct + shared surface |
| views/admin/BackupView.vue | 2 / 0 / 0 | Direct + shared surface |
| views/admin/ChannelMonitorView.vue | 0 / 1 / 0 | Shared primitive inheritance |
| views/admin/ChannelsView.vue | 1 / 1 / 0 | Direct + shared surface |
| views/admin/GroupsView.vue | 4 / 2 / 0 | Direct + shared surface |
| views/admin/PluginsView.vue | 1 / 0 / 0 | Shared primitive inheritance |
| views/admin/PromoCodesView.vue | 3 / 1 / 0 | Direct + shared surface |
| views/admin/ProxiesView.vue | 4 / 3 / 0 | Direct + shared surface |
| views/admin/RedeemView.vue | 3 / 2 / 3 | Direct + shared surface · portal treatment |
| views/admin/RiskControlView.vue | 2 / 0 / 0 | Direct + shared surface |
| views/admin/SettingsView.vue | 3 / 2 / 0 | Direct + shared surface |
| views/admin/SubscriptionsView.vue | 3 / 3 / 0 | Direct + shared surface |
| views/admin/UsersView.vue | 0 / 2 / 1 | Direct + shared surface · portal treatment |
| views/admin/affiliates/AdminAffiliateRecordsTable.vue | 1 / 0 / 0 | Direct + shared surface |
| views/admin/affiliates/AffiliateOfflineWithdrawDialog.vue | 1 / 0 / 0 | Direct + shared surface |
| views/admin/ops/OpsDashboard.vue | 1 / 0 / 0 | Shared primitive inheritance |
| views/admin/ops/components/OpsAlertEventsCard.vue | 1 / 0 / 0 | Direct + shared surface |
| views/admin/ops/components/OpsAlertRulesCard.vue | 1 / 1 / 0 | Direct + shared surface |
| views/admin/ops/components/OpsDashboardHeader.vue | 2 / 0 / 0 | Direct + shared surface |
| views/admin/ops/components/OpsEmailNotificationCard.vue | 1 / 0 / 0 | Direct + shared surface |
| views/admin/ops/components/OpsErrorDetailModal.vue | 1 / 0 / 0 | Direct + shared surface |
| views/admin/ops/components/OpsErrorDetailsModal.vue | 1 / 0 / 0 | Direct + shared surface |
| views/admin/ops/components/OpsRequestDetailsModal.vue | 1 / 0 / 0 | Direct + shared surface |
| views/admin/ops/components/OpsRuntimeSettingsCard.vue | 1 / 0 / 0 | Direct + shared surface |
| views/admin/ops/components/OpsSettingsDialog.vue | 1 / 0 / 0 | Direct + shared surface |
| views/admin/orders/AdminOrdersView.vue | 1 / 0 / 0 | Direct + shared surface |
| views/admin/orders/AdminPaymentPlansView.vue | 0 / 1 / 0 | Shared primitive inheritance |
| views/admin/orders/PlanEditDialog.vue | 1 / 0 / 0 | Direct + shared surface |
| views/user/BatchImageGuideView.vue | 4 / 0 / 2 | Direct + shared surface · portal treatment |
| views/user/KeysView.vue | 2 / 3 / 1 | Direct + shared surface · portal treatment |
| views/user/PaymentView.vue | 1 / 0 / 2 | Direct + shared surface · portal treatment |
| views/user/UserOrdersView.vue | 2 / 0 / 0 | Direct + shared surface |
