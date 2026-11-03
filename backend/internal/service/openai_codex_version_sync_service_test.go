package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

// Mock only the repository operations used by version synchronization.

type codexVersionSyncSettingRepoStub struct {
	SettingRepository // Unimplemented methods must not be called.

	mu        sync.Mutex
	values    map[string]string
	getErr    error
	setErr    error
	updatedAt time.Time
	writes    []string
}

func newCodexVersionSyncSettingRepoStub(values map[string]string) *codexVersionSyncSettingRepoStub {
	if values == nil {
		values = map[string]string{}
	}
	return &codexVersionSyncSettingRepoStub{values: values}
}

func (r *codexVersionSyncSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getErr != nil {
		return "", r.getErr
	}
	return r.values[key], nil
}

func (r *codexVersionSyncSettingRepoStub) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.setErr != nil {
		return r.setErr
	}
	r.values[key] = value
	r.writes = append(r.writes, value)
	return nil
}

func (r *codexVersionSyncSettingRepoStub) syncedWrites() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.writes...)
}

type codexVersionSyncGitHubStub struct {
	GitHubReleaseClient // Unimplemented methods must not be called.

	// A nil latest release simulates unavailable metadata and triggers list fallback.
	latest      *GitHubRelease
	latestErr   error
	latestCalls int

	releases []*GitHubRelease
	err      error
	calls    int
}

func (c *codexVersionSyncGitHubStub) FetchLatestRelease(_ context.Context, _ string) (*GitHubRelease, error) {
	c.latestCalls++
	if c.latestErr != nil {
		return nil, c.latestErr
	}
	return c.latest, nil
}

func (c *codexVersionSyncGitHubStub) FetchRecentReleases(_ context.Context, _ string, _ int) ([]*GitHubRelease, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	return c.releases, nil
}

func newCodexVersionSyncService(
	repo SettingRepository,
	github GitHubReleaseClient,
) *OpenAICodexVersionSyncService {
	return NewOpenAICodexVersionSyncService(repo, &SettingService{}, github, openAICodexVersionSyncInterval)
}

// Accept only stable rust-v tags; unrelated components share this repository.
func TestLatestCodexStableReleaseVersion(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "rusty-v8-v150.4.0"},
		{TagName: "rust-v0.147.0-alpha.4", Prerelease: true},
		{TagName: "rust-v0.146.0"},
		{TagName: "rust-v0.145.0"},
		{TagName: "rust-v0.999.0", Draft: true},
		{TagName: "not-a-tag"},
		nil,
	}

	require.Equal(t, "0.146.0", latestCodexStableReleaseVersion(releases))
	require.Empty(t, latestCodexStableReleaseVersion(nil))
	require.Empty(t, latestCodexStableReleaseVersion([]*GitHubRelease{{TagName: "rusty-v8-v150.4.0"}}))
	// Reject prerelease suffixes even when the release metadata omits Prerelease.
	require.Empty(t, latestCodexStableReleaseVersion([]*GitHubRelease{{TagName: "rust-v0.147.0-alpha.4"}}))
}

func TestOpenAICodexVersionSyncWritesLatestStableVersion(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(nil)
	github := &codexVersionSyncGitHubStub{releases: []*GitHubRelease{
		{TagName: "rust-v0.145.0"},
		{TagName: "rust-v0.146.0"},
	}}

	newCodexVersionSyncService(repo, github).runOnce()

	require.Equal(t, []string{"0.146.0"}, repo.syncedWrites())
}

// Never downgrade an already synchronized version when upstream returns old data.
func TestOpenAICodexVersionSyncNeverMovesBackwards(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(map[string]string{
		SettingKeyOpenAICodexClientVersionSynced: "0.146.0",
	})
	github := &codexVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "rust-v0.145.0"}}}

	newCodexVersionSyncService(repo, github).runOnce()

	require.Empty(t, repo.syncedWrites())
}

func TestOpenAICodexVersionSyncSkippedWhenDisabled(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(map[string]string{
		SettingKeyOpenAICodexVersionAutoSyncEnabled: "false",
	})
	github := &codexVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "rust-v0.146.0"}}}

	newCodexVersionSyncService(repo, github).runOnce()

	require.Zero(t, github.latestCalls, "关闭自动同步后不应请求上游")
	require.Zero(t, github.calls, "关闭自动同步后不应请求上游")
	require.Empty(t, repo.syncedWrites())
}

// A missing or empty switch uses the enabled default.
func TestOpenAICodexVersionSyncEnabledByDefault(t *testing.T) {
	for _, value := range []string{"", "true"} {
		repo := newCodexVersionSyncSettingRepoStub(map[string]string{
			SettingKeyOpenAICodexVersionAutoSyncEnabled: value,
		})
		github := &codexVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "rust-v0.146.0"}}}

		newCodexVersionSyncService(repo, github).runOnce()

		require.Equal(t, []string{"0.146.0"}, repo.syncedWrites(), "开关值 %q", value)
	}
}

// Keep the current version if both fetch paths fail.
func TestOpenAICodexVersionSyncKeepsValueOnFetchError(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(map[string]string{
		SettingKeyOpenAICodexClientVersionSynced: "0.146.0",
	})
	github := &codexVersionSyncGitHubStub{
		latestErr: errors.New("network down"),
		err:       errors.New("network down"),
	}

	newCodexVersionSyncService(repo, github).runOnce()

	require.Equal(t, 1, github.latestCalls)
	require.Equal(t, 1, github.calls)
	require.Empty(t, repo.syncedWrites())
	value, err := repo.GetValue(context.Background(), SettingKeyOpenAICodexClientVersionSynced)
	require.NoError(t, err)
	require.Equal(t, "0.146.0", value)
}

// Prefer the latest-release endpoint to avoid scanning large prerelease lists.
func TestOpenAICodexVersionSyncUsesLatestReleaseEndpoint(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(nil)
	github := &codexVersionSyncGitHubStub{
		latest: &GitHubRelease{TagName: "rust-v0.146.0"},
		// A different list result proves that the primary endpoint takes precedence.
		releases: []*GitHubRelease{{TagName: "rust-v0.145.0"}},
	}

	newCodexVersionSyncService(repo, github).runOnce()

	require.Equal(t, 1, github.latestCalls)
	require.Zero(t, github.calls, "主路径可用时不应再拉列表页")
	require.Equal(t, []string{"0.146.0"}, repo.syncedWrites())
}

// Fall back to the release list when latest has no usable client release.
func TestOpenAICodexVersionSyncFallsBackToReleaseList(t *testing.T) {
	tests := []struct {
		name      string
		latest    *GitHubRelease
		latestErr error
	}{
		// The newest release can belong to another component in the same repository.
		{name: "latest 属于其他 tag 家族", latest: &GitHubRelease{TagName: "rusty-v8-v150.4.0"}},
		// Filter draft and prerelease results even if endpoint semantics change.
		{name: "latest 是预发布", latest: &GitHubRelease{TagName: "rust-v0.147.0-alpha.4", Prerelease: true}},
		{name: "latest 抓取失败", latestErr: errors.New("network down")},
		// Treat an empty release as unavailable rather than panicking.
		{name: "latest 为空", latest: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newCodexVersionSyncSettingRepoStub(nil)
			github := &codexVersionSyncGitHubStub{
				latest:    tt.latest,
				latestErr: tt.latestErr,
				releases: []*GitHubRelease{
					{TagName: "rust-v0.147.0-alpha.4", Prerelease: true},
					{TagName: "rust-v0.146.0"},
					{TagName: "rust-v0.145.0"},
				},
			}

			newCodexVersionSyncService(repo, github).runOnce()

			require.Equal(t, 1, github.latestCalls)
			require.Equal(t, 1, github.calls)
			require.Equal(t, []string{"0.146.0"}, repo.syncedWrites())
		})
	}
}

// Apply the same tag-prefix and version validation to both fetch paths.
func TestOpenAICodexVersionSyncLatestSharesFiltering(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(nil)
	github := &codexVersionSyncGitHubStub{
		// Reject malformed tags after stripping exactly one rust-v prefix.
		latest:   &GitHubRelease{TagName: "rust-vv0.99.0"},
		releases: []*GitHubRelease{{TagName: "rust-v0.146.0"}},
	}

	newCodexVersionSyncService(repo, github).runOnce()

	require.Equal(t, []string{"0.146.0"}, repo.syncedWrites())
}

// Do not start an idle goroutine when dependencies are missing.
func TestOpenAICodexVersionSyncStartRequiresDependencies(t *testing.T) {
	require.NotPanics(t, func() {
		svc := NewOpenAICodexVersionSyncService(nil, nil, nil, openAICodexVersionSyncInterval)
		svc.Start()
		svc.Stop()
	})
}

// Mock version settings read through GetMultiple.

type codexVersionSettingRepoStub struct {
	SettingRepository // Unimplemented methods must not be called.

	values map[string]string
	err    error
}

func (r *codexVersionSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = r.values[key]
	}
	return out, nil
}

// Canonical identity resolution first reads the full custom User-Agent.
func (r *codexVersionSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	return r.values[key], nil
}

// Explicit version settings take precedence over synchronized and bundled versions.
func TestGetOpenAICodexClientVersionPriority(t *testing.T) {
	tests := []struct {
		name     string
		override string
		synced   string
		want     string
	}{
		{name: "面板覆写优先", override: "0.150.0", synced: "0.146.0", want: "0.150.0"},
		{name: "覆写为空时用同步值", synced: "0.146.0", want: "0.146.0"},
		{name: "两者皆空时用内置常量", want: codexCLIVersion},
		{name: "非法覆写回退同步值", override: "latest", synced: "0.146.0", want: "0.146.0"},
		{name: "非法同步值回退内置常量", synced: "not-a-version", want: codexCLIVersion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
				SettingKeyOpenAICodexClientVersion:       tt.override,
				SettingKeyOpenAICodexClientVersionSynced: tt.synced,
			}}, nil)

			require.Equal(t, tt.want, svc.GetOpenAICodexClientVersion(context.Background()))
		})
	}
}

// Fall back to the bundled version on read errors.
func TestGetOpenAICodexClientVersionFallsBackOnError(t *testing.T) {
	svc := NewSettingService(&codexVersionSettingRepoStub{err: errors.New("db down")}, nil)
	require.Equal(t, codexCLIVersion, svc.GetOpenAICodexClientVersion(context.Background()))
}

// Without a custom UA, retain the local CLI profile and use the selected engine version.
func TestGetOpenAICodexCanonicalUserAgentBuildsFromVersion(t *testing.T) {
	svc := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
		SettingKeyOpenAICodexClientVersionSynced: "0.200.1",
	}}, nil)

	require.Equal(t,
		buildCodexCLIUserAgent("0.200.1"),
		svc.GetOpenAICodexCanonicalUserAgent(context.Background()),
	)
}

// Keep custom client and platform fields while rebuilding the engine version.
func TestGetOpenAICodexCanonicalUserAgentRebuildsPanelUAVersion(t *testing.T) {
	t.Run("陈旧面板 UA 跟随生效版本", func(t *testing.T) {
		svc := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
			// Preserve compatibility with deployments using the historical placeholder.
			SettingKeyOpenAICodexUserAgent:           "codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color",
			SettingKeyOpenAICodexClientVersionSynced: "0.200.1",
		}}, nil)

		require.Equal(t,
			"codex_cli_rs/0.200.1 (Ubuntu 22.4.0; x86_64) xterm-256color",
			svc.GetOpenAICodexCanonicalUserAgent(context.Background()),
		)
	})

	t.Run("自定义指纹原样保留", func(t *testing.T) {
		svc := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
			SettingKeyOpenAICodexUserAgent:           "codex_cli_rs/0.140.0 (Mac OS X 15.1.0; arm64) iTerm.app",
			SettingKeyOpenAICodexClientVersionSynced: "0.200.1",
		}}, nil)

		require.Equal(t,
			"codex_cli_rs/0.200.1 (Mac OS X 15.1.0; arm64) iTerm.app",
			svc.GetOpenAICodexCanonicalUserAgent(context.Background()),
		)
	})

	t.Run("TUI UA 的首尾两个版本号同时更新", func(t *testing.T) {
		svc := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
			SettingKeyOpenAICodexUserAgent:           "codex-tui/0.146.1 (Ubuntu 22.4.0; x86_64) WindowsTerminal (codex-tui; 0.146.1)",
			SettingKeyOpenAICodexClientVersionSynced: "0.200.1",
		}}, nil)

		require.Equal(t,
			"codex-tui/0.200.1 (Ubuntu 22.4.0; x86_64) WindowsTerminal (codex-tui; 0.200.1)",
			svc.GetOpenAICodexCanonicalUserAgent(context.Background()),
		)
	})

	// Explicit version settings must continue to override synchronized values.
	t.Run("面板版本号覆写优先", func(t *testing.T) {
		svc := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
			SettingKeyOpenAICodexUserAgent:           "codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color",
			SettingKeyOpenAICodexClientVersion:       "0.150.0",
			SettingKeyOpenAICodexClientVersionSynced: "0.200.1",
		}}, nil)

		require.Equal(t,
			"codex_cli_rs/0.150.0 (Ubuntu 22.4.0; x86_64) xterm-256color",
			svc.GetOpenAICodexCanonicalUserAgent(context.Background()),
		)
	})

	// Unrecognized client identities fall back to the canonical profile.
	t.Run("非 Codex 形态回退", func(t *testing.T) {
		svc := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
			SettingKeyOpenAICodexUserAgent: "not-a-codex-client",
		}}, nil)

		require.Equal(t, codexCLIUserAgent, svc.GetOpenAICodexCanonicalUserAgent(context.Background()))
	})
}

func (r *codexVersionSyncSettingRepoStub) Get(_ context.Context, key string) (*Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getErr != nil {
		return nil, r.getErr
	}
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value, UpdatedAt: r.updatedAt}, nil
}

// Skip fresh startup synchronization to avoid repeated release requests.
func TestOpenAICodexVersionSyncInitialSkipsWhenRecentlySynced(t *testing.T) {
	repo := newCodexVersionSyncSettingRepoStub(map[string]string{
		SettingKeyOpenAICodexClientVersionSynced: "0.146.0",
	})
	repo.updatedAt = time.Now().Add(-time.Hour)
	github := &codexVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "rust-v0.147.0"}}}

	newCodexVersionSyncService(repo, github).runInitial()

	require.Zero(t, github.calls, "同步值仍在周期内时不应请求上游")
	require.Empty(t, repo.syncedWrites())
}

func TestOpenAICodexVersionSyncInitialRunsWhenStaleOrMissing(t *testing.T) {
	t.Run("同步值已过期", func(t *testing.T) {
		repo := newCodexVersionSyncSettingRepoStub(map[string]string{
			SettingKeyOpenAICodexClientVersionSynced: "0.146.0",
		})
		repo.updatedAt = time.Now().Add(-7 * time.Hour)
		github := &codexVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "rust-v0.147.0"}}}

		newCodexVersionSyncService(repo, github).runInitial()

		require.Equal(t, 1, github.calls)
		require.Equal(t, []string{"0.147.0"}, repo.syncedWrites())
	})

	// A fresh deployment without a synchronized version must fetch immediately.
	t.Run("尚无同步值", func(t *testing.T) {
		repo := newCodexVersionSyncSettingRepoStub(nil)
		repo.updatedAt = time.Now()
		github := &codexVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "rust-v0.146.0"}}}

		newCodexVersionSyncService(repo, github).runInitial()

		require.Equal(t, 1, github.calls)
		require.Equal(t, []string{"0.146.0"}, repo.syncedWrites())
	})
}

// Compare numeric components; lexical ordering incorrectly ranks 0.99 above 0.146.
func TestCodexVersionComparisonIsNumericNotLexical(t *testing.T) {
	require.Greater(t, CompareVersions("0.146.0", "0.99.0"), 0)

	require.Equal(t, "0.146.0", latestCodexStableReleaseVersion([]*GitHubRelease{
		{TagName: "rust-v0.99.0"},
		{TagName: "rust-v0.146.0"},
	}))

	repo := newCodexVersionSyncSettingRepoStub(map[string]string{
		SettingKeyOpenAICodexClientVersionSynced: "0.146.0",
	})
	github := &codexVersionSyncGitHubStub{releases: []*GitHubRelease{{TagName: "rust-v0.99.0"}}}

	newCodexVersionSyncService(repo, github).runOnce()

	require.Empty(t, repo.syncedWrites(), "0.99.0 低于已同步的 0.146.0，不得写入")
}

func TestGetOpenAICodexCanonicalUserAgentOutboundIdentity(t *testing.T) {
	codexCanonicalUAMu.RLock()
	previous := codexCanonicalUAResolver
	codexCanonicalUAMu.RUnlock()
	t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(previous) })

	for _, version := range []struct {
		name, manual, synced, want string
	}{
		{"manual", "0.154.0", "0.155.1", "0.154.0"},
		{"synced", "", "0.155.1", "0.155.1"},
		{"invalid_manual", "latest", "0.155.1", "0.155.1"},
		{"default", "", "", codexCLIVersion},
		{"invalid_versions", "latest", "invalid", codexCLIVersion},
	} {
		for _, ua := range []struct {
			name, value, client, suffix string
		}{
			{name: "browser", value: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36"},
			{name: "arbitrary", value: "not-a-codex-client"},
			{name: "unknown_client", value: "custom/0.140.0 (Linux)"},
			{name: "empty"},
			{name: "missing_version", value: "codex_cli_rs/"},
			{name: "crlf", value: "codex_cli_rs/0.140.0 (Linux)\r\nX-Injected: value"},
			{name: "nul", value: "codex_cli_rs/0.140.0 (Linux\x00)"},
			{name: "del", value: "codex_cli_rs/0.140.0 terminal\x7f"},
			{name: "leading_newline", value: "\ncodex_cli_rs/0.140.0 (Linux)"},
			{name: "trailing_newline", value: "codex_cli_rs/0.140.0 (Linux)\n"},
			{name: "valid_suffix", value: "codex_cli_rs/0.140.0 (Mac OS X 15.1.0; arm64) iTerm.app", client: "codex_cli_rs", suffix: " (Mac OS X 15.1.0; arm64) iTerm.app"},
			{name: "valid_tab", value: "codex_cli_rs/0.140.0 (Linux)\tterminal", client: "codex_cli_rs", suffix: " (Linux)\tterminal"},
			{name: "recognized_trailer", value: "custom/0.140.0 (Linux) (codex-tui; 0.140.0)", client: "codex-tui", suffix: " (Linux) (codex-tui; " + version.want + ")"},
		} {
			t.Run(version.name+"/"+ua.name, func(t *testing.T) {
				svc := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
					SettingKeyOpenAICodexUserAgent:           ua.value,
					SettingKeyOpenAICodexClientVersion:       version.manual,
					SettingKeyOpenAICodexClientVersionSynced: version.synced,
				}}, nil)
				wantCanonicalUA := buildCodexCLIUserAgent(version.want)
				wantUA := wantCanonicalUA
				wantOriginator := openai.CodexDefaultOriginator
				if ua.client != "" {
					wantUA = ua.client + "/" + version.want + ua.suffix
					wantCanonicalUA = wantUA
					wantOriginator = ua.client
				}
				wantOriginator, wantUA, _ = openai.NormalizeCodexClientIdentityToCLI(wantOriginator, wantUA)
				SetCodexCanonicalUserAgentResolver(func() string {
					return svc.GetOpenAICodexCanonicalUserAgent(context.Background())
				})
				require.Equal(t, wantCanonicalUA, svc.GetOpenAICodexCanonicalUserAgent(context.Background()))
				identity := resolveCodexOutboundIdentity("")
				require.Equal(t, wantUA, identity.userAgent)
				require.Equal(t, wantOriginator, identity.originator)
				require.Equal(t, version.want, identity.version)
			})
		}
	}
}
