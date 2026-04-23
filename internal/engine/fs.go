package engine

import "os"

var osStat = os.Stat

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
