package cli

import (
	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/usererr"
)

type createRequest = lifecycle.Request

func withRepo(req createRequest, flags createFlags) (createRequest, error) {
	if !req.Sync {
		if flags.DirSet {
			return createRequest{}, usererr.New(
				"--dir requires --sync",
				"pass --sync to copy that repository",
			)
		}
		return req, nil
	}
	root, name, err := lifecycle.ResolveRepo(flags.Dir, flags.DirSet)
	if err != nil {
		return createRequest{}, err
	}
	req.Root = root
	req.Repo = name
	return req, nil
}
