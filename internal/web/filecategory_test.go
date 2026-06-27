package web

import "testing"

func TestFileCategory(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		// image
		{"photo.jpg", "image"},
		{"icon.PNG", "image"},
		{"anim.gif", "image"},
		{"graphic.svg", "image"},
		// video
		{"movie.mp4", "video"},
		{"clip.MKV", "video"},
		{"screen.mov", "video"},
		// audio
		{"song.mp3", "audio"},
		{"lossless.FLAC", "audio"},
		{"podcast.ogg", "audio"},
		// archive
		{"backup.tar.gz", "archive"},
		{"data.zip", "archive"},
		{"dist.tar.zst", "archive"},
		// code
		{"main.go", "code"},
		{"app.js", "code"},
		{"config.yaml", "code"},
		{"schema.json", "code"},
		{"script.sh", "code"},
		// pdf
		{"report.pdf", "pdf"},
		{"invoice.PDF", "pdf"},
		// doc
		{"readme.md", "doc"},
		{"notes.txt", "doc"},
		{"letter.docx", "doc"},
		// sheet
		{"data.csv", "sheet"},
		{"numbers.xlsx", "sheet"},
		// fallback
		{"binary.bin", "file"},
		{"noextension", "file"},
		{".", "file"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := fileCategory(tc.name)
			if got != tc.want {
				t.Errorf("fileCategory(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}
