package bc

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"github.com/google/go-github/v31/github"
)

func LoadGenesis() (genesis Genesis, err error) {
	file, err := ioutil.ReadFile("Genesis.json")
	if err != nil {
		//TODO Download genesis
		repo := github.RepositoriesService{}

		context := context.Background()
		repo.DownloadContents(context, "garcticman", "simple_chain", "Genesis.json", &github.RepositoryContentGetOptions{})
	}

	err = json.Unmarshal(file, genesis)

	return
}
