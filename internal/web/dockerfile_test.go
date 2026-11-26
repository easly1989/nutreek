package web

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The Docker build makes tailwind.css in a stage that has only the files COPYed into
// it. A folder that tailwind.config.js reads for classes but the stage lacks gives a
// bundle without them: the day swipe hint was missing in the image for that reason.
func TestDockerfileCopiesEveryTailwindContentFolder(t *testing.T) {
	cfg, err := os.ReadFile("../../web/tailwind/tailwind.config.js")
	if err != nil {
		t.Fatal(err)
	}
	docker, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	stage, _, _ := strings.Cut(string(docker), "AS go-builder")
	paths := regexp.MustCompile(`"\./(internal/[^"*{]+/)`).FindAllStringSubmatch(string(cfg), -1)
	if len(paths) == 0 {
		t.Fatal("no content folder found in tailwind.config.js")
	}
	for _, m := range paths {
		dir := strings.TrimSuffix(m[1], "/")
		if !strings.Contains(stage, "COPY "+dir+" ") {
			t.Errorf("tailwind.config.js reads %s but the tailwind stage of the Dockerfile does not COPY it", dir)
		}
	}
}
