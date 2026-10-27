package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"html"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/photo"
)

// upload posts data as the photo of recipe id with the CSRF token of its
// page, as the page's script does when script is set.
func (b *browser) upload(id int64, data []byte, script bool) (int, string, string) {
	b.t.Helper()
	page := "/recipes/" + strconv.FormatInt(id, 10)
	_, _, body := b.get(page)
	m := csrfField.FindStringSubmatch(body)
	if m == nil {
		b.t.Fatalf("no CSRF token on %s", page)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("csrf_token", html.UnescapeString(m[1]))
	fw, _ := mw.CreateFormFile("photo", "photo.jpg")
	_, _ = fw.Write(data)
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, b.srv.URL+page+"/photo", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", b.srv.URL)
	if script {
		req.Header.Set("X-Requested-With", "fetch")
	}
	resp, err := b.c.Do(req)
	if err != nil {
		b.t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Location"), string(out)
}

// fetchImage GETs path and returns the status and the decoded picture size.
func (b *browser) fetchImage(path string, header http.Header) (*http.Response, image.Point) {
	b.t.Helper()
	req, _ := http.NewRequest(http.MethodGet, b.srv.URL+path, nil)
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := b.c.Do(req)
	if err != nil {
		b.t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return resp, image.Point{}
	}
	if bytes.Contains(data, []byte("Exif")) {
		b.t.Errorf("%s still carries EXIF", path)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "jpeg" {
		b.t.Fatalf("%s: not a JPEG (%q, %v)", path, format, err)
	}
	return resp, image.Pt(cfg.Width, cfg.Height)
}

func testPicture(t *testing.T, w, h int, format string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = byte(i)
	}
	var buf bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&buf, img)
	case "gif":
		err = gif.Encode(&buf, img, nil)
	default:
		if err := jpeg.Encode(&buf, img, nil); err != nil {
			t.Fatal(err)
		}
		// An EXIF segment with a position, right after the SOI.
		exif := append([]byte("Exif\x00\x00"), "GPS 45.46N 9.19E"...)
		seg := binary.BigEndian.AppendUint16([]byte{0xFF, 0xE1}, uint16(len(exif)+2))
		data := buf.Bytes()
		return append(append(append([]byte{0xFF, 0xD8}, seg...), exif...), data[2:]...)
	}
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func photoUser(t *testing.T, a *App, email string) (*browser, int64) {
	t.Helper()
	b := signIn(t, a, email)
	u, err := a.Users.GetByEmail(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	return b, u.ID
}

func TestRecipePhoto(t *testing.T) {
	a := newTestApp(t)
	alice, aliceID := photoUser(t, a, "alice@example.test")
	bob, _ := photoUser(t, a, "bob@example.test")
	ctx := context.Background()
	priv, _ := a.Recipes.Create(ctx, aliceID, "Private bowl", nil, 1, false)
	pub, _ := a.Recipes.Create(ctx, aliceID, "Public bowl", nil, 1, true)
	page := "/recipes/" + strconv.FormatInt(pub, 10)

	// Bob sees no empty box on a public recipe without a photo; Alice does.
	if _, _, body := bob.get(page); strings.Contains(body, "data-photo-input") || strings.Contains(body, "data-photo") {
		t.Error("a viewer sees the photo box of a recipe without photo")
	}
	if _, _, body := alice.get(page); !strings.Contains(body, "Add a photo") {
		t.Error("the editor gets no Add a photo box")
	}

	// A phone-sized JPEG with EXIF, from the script: stored at 1200 px, no EXIF.
	if code, _, body := alice.upload(pub, testPicture(t, 2000, 1500, "jpeg"), true); code != http.StatusNoContent {
		t.Fatalf("JPEG upload: %d %s", code, body)
	}
	resp, got := alice.fetchImage(page+"/photo", nil)
	if got != image.Pt(1200, 900) {
		t.Errorf("photo %v, want 1200x900", got)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" || !strings.HasPrefix(resp.Header.Get("Cache-Control"), "private") {
		t.Errorf("ETag %q, Cache-Control %q", etag, resp.Header.Get("Cache-Control"))
	}
	if _, got := alice.fetchImage(page+"/photo/thumb", nil); got != image.Pt(photo.ThumbSide, photo.ThumbSide) {
		t.Errorf("thumb %v, want %dx%[2]d", got, photo.ThumbSide)
	}
	if resp, _ := alice.fetchImage(page+"/photo", http.Header{"If-None-Match": {etag}}); resp.StatusCode != http.StatusNotModified {
		t.Errorf("revalidation: %d, want 304", resp.StatusCode)
	}
	v := strings.Trim(etag, `"`)
	if _, _, body := alice.get(page); !strings.Contains(body, `/photo?v=`+v) || !strings.Contains(body, "Change photo") {
		t.Error("the recipe page does not show the photo with its version")
	}
	if _, _, body := alice.get("/recipes"); !strings.Contains(body, `/photo/thumb?v=`+v) {
		t.Error("the list does not show the thumbnail")
	}

	// A PNG from a plain form: back to the page.
	if code, loc, _ := alice.upload(priv, testPicture(t, 300, 200, "png"), false); code != http.StatusSeeOther || loc != "/recipes/"+strconv.FormatInt(priv, 10) {
		t.Errorf("PNG upload: %d to %q", code, loc)
	}
	if _, got := alice.fetchImage("/recipes/"+strconv.FormatInt(priv, 10)+"/photo", nil); got != image.Pt(300, 200) {
		t.Errorf("PNG photo %v, want 300x200", got)
	}

	// Refused: another format, and a file over 10 MB.
	if code, _, body := alice.upload(pub, testPicture(t, 10, 10, "gif"), true); code != http.StatusUnprocessableEntity || !strings.Contains(body, "use a JPEG or PNG") {
		t.Errorf("GIF upload: %d %q", code, body)
	}
	if code, _, _ := alice.upload(pub, make([]byte, maxPhotoBytes+1), true); code != http.StatusRequestEntityTooLarge {
		t.Errorf("11 MB upload: %d, want 413", code)
	}

	// Bob may see the public recipe's photo, not the private one's, and may
	// change neither.
	if resp, _ := bob.fetchImage(page+"/photo", nil); resp.StatusCode != http.StatusOK {
		t.Errorf("Bob reads the public photo: %d", resp.StatusCode)
	}
	if resp, _ := bob.fetchImage("/recipes/"+strconv.FormatInt(priv, 10)+"/photo/thumb", nil); resp.StatusCode == http.StatusOK {
		t.Error("Bob reads the private recipe's photo")
	}
	if code, _, _ := bob.upload(pub, testPicture(t, 20, 20, "png"), true); code != http.StatusForbidden {
		t.Errorf("Bob uploads to Alice's recipe: %d, want 403", code)
	}
	if code, _ := bob.post(page, page+"/photo/delete", nil); code != http.StatusForbidden {
		t.Errorf("Bob removes Alice's photo: %d, want 403", code)
	}

	// Remove: the photo is gone.
	if code, loc := alice.post(page, page+"/photo/delete", nil); code != http.StatusSeeOther || loc != page {
		t.Errorf("remove: %d to %q", code, loc)
	}
	if resp, _ := alice.fetchImage(page+"/photo", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("after remove: %d, want 404", resp.StatusCode)
	}
}

// In the demo the page's script keeps uploads under the body limit.
func TestDemoRecipePhoto(t *testing.T) {
	a := newDemoApp(t, DemoLimits{MaxBody: 1 << 20})
	b := newBrowser(t, a)
	uid := enter(t, a, b)
	id, err := a.Recipes.Create(context.Background(), uid, "Demo bowl", nil, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, body := b.get("/recipes/" + strconv.FormatInt(id, 10)); !strings.Contains(body, `data-photo-max="`+strconv.Itoa(1<<20-8<<10)+`"`) {
		t.Error("the page does not tell the script the demo's limit")
	}
	if code, _, body := b.upload(id, testPicture(t, 800, 600, "png"), true); code != http.StatusNoContent {
		t.Errorf("demo upload: %d %s", code, body)
	}
}
