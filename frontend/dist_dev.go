//go:build !prod

package frontend

import "io/fs"

func Dist() (fs.FS, bool, error) {
	return nil, false, nil
}
