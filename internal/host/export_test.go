package host

import "github.com/exisz/bitgit/internal/config"

// NewBitbucketDCForTest creates a Bitbucket DC host with the given remote URL.
// Exported for use in tests only.
func NewBitbucketDCForTest(remoteURL string, cfg *config.Config) (Host, error) {
	return newBitbucketDC(remoteURL, cfg)
}
