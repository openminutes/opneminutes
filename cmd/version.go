package cmd

const (
	defaultVersion = "dev"
	defaultCommit  = "none"
)

var (
	version = defaultVersion
	commit  = defaultCommit
)

func buildVersion() string {
	if version == "" || version == defaultVersion || commit == "" || commit == defaultCommit {
		return defaultVersion
	}

	if len(commit) < 8 {
		return defaultVersion
	}

	shortCommit := commit
	if len(shortCommit) > 8 {
		shortCommit = shortCommit[:8]
	}

	return version + "-" + shortCommit
}
