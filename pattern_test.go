package govc

import (
	"path/filepath"
	"testing"
)

func TestPatternMatching(t *testing.T) {
	testPaths := []string{
		"config.json",
		"frontend/src/components/Button.js",
		"frontend/src/components/Modal.js",
		"backend/src/controllers/auth.go",
		"config/database.json",
		"config/app.yaml",
	}
	
	t.Run("FilepathMatch", func(t *testing.T) {
		for _, path := range testPaths {
			matched, err := filepath.Match("*.js", path)
			t.Logf("filepath.Match('*.js', '%s') = %v, %v", path, matched, err)
		}
		
		for _, path := range testPaths {
			matched, err := filepath.Match("*.js", filepath.Base(path))
			t.Logf("filepath.Match('*.js', filepath.Base('%s')) = %v, %v", path, matched, err)
		}
	})
}