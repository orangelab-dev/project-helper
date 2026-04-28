//go:build prod

package frontend

import (
	"embed"
	"io/fs"
)

//go:embed dist
var embeddedDist embed.FS

func Dist() (fs.FS, bool, error) {
	dist, err := fs.Sub(embeddedDist, "dist")
	if err != nil {
		return nil, true, err
	}
	return dist, true, nil
}
