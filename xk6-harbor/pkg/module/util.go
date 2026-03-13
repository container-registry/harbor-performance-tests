package module

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/grafana/sobek"
	"go.k6.io/k6/js/common"
	"oras.land/oras-go/v2/content/file"
)

func newLocalStore(rt *sobek.Runtime, name string) (string, *file.Store) {
	rootPath := filepath.Join(DefaultRootPath, name)

	store, err := file.New(rootPath)
	Check(rt, err)

	return rootPath, store
}

func isDigest(reference string) bool {
	i := strings.Index(reference, ":")
	return i > 0 && i+1 != len(reference)
}

func getDistrubtionRef(projectName, repositoryName, reference string) string {
	if isDigest(reference) {
		return fmt.Sprintf("%s/%s@%s", projectName, repositoryName, reference)
	}

	return fmt.Sprintf("%s/%s:%s", projectName, repositoryName, reference)
}

func Check(rt *sobek.Runtime, err error) {
	if err == nil {
		return
	}

	common.Throw(rt, err)
}

func Checkf(rt *sobek.Runtime, err error, format string, a ...interface{}) {
	if err == nil {
		return
	}

	common.Throw(
		rt,
		fmt.Errorf("%s, error: %s", fmt.Sprintf(format, a...), err),
	)
}

func Throwf(rt *sobek.Runtime, format string, a ...interface{}) {
	common.Throw(rt, fmt.Errorf(format, a...))
}

func ExportTo(rt *sobek.Runtime, target interface{}, args ...sobek.Value) {
	if len(args) > 0 {
		if err := rt.ExportTo(args[0], target); err != nil {
			common.Throw(rt, err)
		}
	}
}

func IDFromLocation(rt *sobek.Runtime, loc string) int64 {
	parts := strings.Split(loc, "/")

	id, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	Check(rt, err)

	return id
}

func NameFromLocation(loc string) string {
	parts := strings.Split(loc, "/")

	return parts[len(parts)-1]
}
