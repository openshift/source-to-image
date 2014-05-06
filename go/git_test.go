package sti

import (
	"testing"
)

func TestValidCloneSpec(t *testing.T) {
	scenarios := []string{"git@github.com:user/repo.git",
		"git://github.com/user/repo.git",
		"git://github.com/user/repo",
		"http://github.com/user/repo.git",
		"http://github.com/user/repo",
		"https://github.com/user/repo.git",
		"https://github.com/user/repo",
		"file:///home/user/code/repo.git",
		"/home/user/code/repo.git",
	}

	for _, scenario := range scenarios {
		result := validCloneSpec(scenario, false)
		if result == false {
			t.Error(scenario)
		}
	}
}
