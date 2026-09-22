package history

import "fmt"

type RemoteTagMetadataError struct {
	Tag     string
	Problem string
	Err     error
}

func (e *RemoteTagMetadataError) Error() string { return e.Err.Error() }

func (e *RemoteTagMetadataError) Unwrap() error { return e.Err }

func remoteTagMetadataError(tag, problem, detail string) error {
	return &RemoteTagMetadataError{
		Tag:     tag,
		Problem: problem,
		Err:     fmt.Errorf("%w: tag %q %s", errRemoteTagMetadata, tag, detail),
	}
}
