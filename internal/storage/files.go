package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	corestorage "github.com/OpenNSW/core/storage"
)

// File is what the storage that holds a file says about it. Its type and
// extension are the ones it was uploaded with; its content is not read.
type File struct {
	// Extension is the file's, with its dot (".pdf"), or empty when it has
	// none.
	Extension string
	// MediaType is the type the file was stored with.
	MediaType string
}

// Files describes the files behind stored values.
type Files interface {
	// Describe returns the file stored under value. A value no file is
	// stored under, such as one whose upload was never made, is an error
	// wrapping corestorage.ErrNotFound.
	Describe(ctx context.Context, value string) (File, error)
}

// serviceFiles describes a file from its download, so the download route and
// Describe say the same about it.
type serviceFiles struct {
	svc Service
}

func (f serviceFiles) Describe(ctx context.Context, value string) (File, error) {
	dl, err := downloadOf(ctx, f.svc, value)
	// Behind a proxy the owning service's 404 is a file it does not hold.
	var rejection *RemoteRejection
	if errors.As(err, &rejection) && rejection.StatusCode == http.StatusNotFound {
		return File{}, fmt.Errorf("storage proxy: describe: %w", corestorage.ErrNotFound)
	}
	if err != nil {
		return File{}, err
	}
	return File{Extension: dl.Extension, MediaType: dl.MimeType}, nil
}
