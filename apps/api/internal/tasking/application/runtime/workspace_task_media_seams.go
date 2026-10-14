package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"

	"ideaboard/api/internal/media/storage/infrastructure/remote"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/tasking/workspacetasks"
)

// WorkspaceTaskResultBytesFetcher is the production remote-result seam. The
// downloader owns SSRF protection, outbound timeout and response size caps;
// this adapter only turns its temporary validated image file into bytes.
type WorkspaceTaskResultBytesFetcher struct{}

var _ WorkspaceResultBytesFetcher = WorkspaceTaskResultBytesFetcher{}

type workspaceResultAllowHostsKey struct{}

func WithWorkspaceResultAllowHosts(ctx context.Context, hosts []string) context.Context {
	return context.WithValue(ctx, workspaceResultAllowHostsKey{}, append([]string(nil), hosts...))
}

func WorkspaceResultAllowHostsFromCtx(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	hosts, _ := ctx.Value(workspaceResultAllowHostsKey{}).([]string)
	return append([]string(nil), hosts...)
}

func (WorkspaceTaskResultBytesFetcher) FetchResultBytes(ctx context.Context, _ *workspacetasks.WorkspaceTask, ref workspacetasks.OutputRef) ([]byte, error) {
	allowHosts := WorkspaceResultAllowHostsFromCtx(ctx)
	if len(allowHosts) == 0 {
		allowHosts = outbound.AllowHosts(ctx)
	}
	// Closed-world gate BEFORE any network: the transport SSRF guard
	// below still applies, but on its own it permits any public host.
	// Provider result egress stays pinned to the executor-supplied
	// allowlist (exact host, or exact host:port); anything else fails
	// closed here and is never fetched.
	rawURL := strings.TrimSpace(ref.URL)
	if err := CheckWorkspaceResultHostAllowlist(rawURL, allowHosts); err != nil {
		return nil, err
	}
	file, err := remote.DownloadImageWithLimit(ctx, rawURL, allowHosts, workspacemedia.MaxUploadSizeBytes)
	if err != nil {
		return nil, err
	}
	defer file.Cleanup()
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind remote image: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(file, workspacemedia.MaxUploadSizeBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read remote image: %w", err)
	}
	if int64(len(data)) > workspacemedia.MaxUploadSizeBytes {
		return nil, fmt.Errorf("remote image exceeds size limit")
	}
	return data, nil
}

// CheckWorkspaceResultHostAllowlist enforces the closed-world egress gate
// for provider result downloads. The URL must be http(s) without userinfo
// and its host must match the allowlist exactly (bare host matches any
// port, host:port matches exactly); entry shapes mirror the outbound
// policy normalization (bare host, host:port, or full URL). An empty
// allowlist fails closed: without operator-provided egress hosts there is
// nothing safe to fetch. Errors name only the host (never the full URL or
// payload) so operators know which host to allowlist.
func CheckWorkspaceResultHostAllowlist(rawURL string, allowHosts []string) error {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return errors.New("workspace provider result is empty")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || strings.TrimSpace(parsed.Hostname()) == "" {
		return errors.New("workspace provider result URL is invalid")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("workspace provider result scheme %q is not allowed", strings.ToLower(parsed.Scheme))
	}
	if parsed.User != nil {
		return errors.New("workspace provider result URL is invalid")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	port := strings.TrimSpace(parsed.Port())
	if port == "" {
		if strings.ToLower(parsed.Scheme) == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}
	matched, configured := matchWorkspaceResultAllowHost(host, port, allowHosts)
	if !configured {
		return errors.New("workspace provider result download allowlist is not configured")
	}
	if !matched {
		return fmt.Errorf("workspace provider result host %q is not allowlisted", host)
	}
	return nil
}

// matchWorkspaceResultAllowHost reports whether host matches the allowlist
// and whether the allowlist carries any usable entry. Bare-host entries
// match any port; host:port entries match exactly.
func matchWorkspaceResultAllowHost(host, port string, allowHosts []string) (matched, configured bool) {
	for _, raw := range allowHosts {
		entryHost, entryPort := normalizeWorkspaceResultAllowEntry(raw)
		if entryHost == "" {
			continue
		}
		configured = true
		if entryHost != host {
			continue
		}
		if entryPort == "" || entryPort == port {
			return true, true
		}
	}
	return false, configured
}

// normalizeWorkspaceResultAllowEntry splits one allowlist entry into its
// normalized (host, port) pair, mirroring the outbound policy and safehttp
// entry shapes. Unusable entries read as ("", "") and are skipped.
func normalizeWorkspaceResultAllowEntry(raw string) (string, string) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", ""
	}
	if parsed, err := url.Parse(value); err == nil && strings.TrimSpace(parsed.Hostname()) != "" {
		host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
		if host == "" {
			return "", ""
		}
		port := strings.TrimSpace(parsed.Port())
		if port == "" {
			switch strings.ToLower(parsed.Scheme) {
			case "http":
				port = "80"
			case "https":
				port = "443"
			}
		}
		return host, port
	}
	if strings.Contains(value, "/") {
		return "", ""
	}
	if entryHost, entryPort, err := net.SplitHostPort(value); err == nil {
		entryHost = strings.ToLower(strings.TrimSuffix(strings.Trim(strings.TrimSpace(entryHost), "[]"), "."))
		if entryHost == "" {
			return "", ""
		}
		return entryHost, strings.TrimSpace(entryPort)
	}
	host := strings.ToLower(strings.TrimSuffix(strings.Trim(strings.TrimSpace(value), "[]"), "."))
	if host == "" {
		return "", ""
	}
	return host, ""
}
