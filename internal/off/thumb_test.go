package off

import "testing"

func TestThumbURL(t *testing.T) {
	const img = "https://images.openfoodfacts.org/images/products/801/234/567/8907/front_it.3.200.jpg"
	cases := map[string]string{
		img: img,
		// the dumps' old host: same images, the only host the CSP allows
		"https://static.openfoodfacts.org/images/products/801/234/567/8907/front_it.3.200.jpg": img,
		"http://images.openfoodfacts.org/images/products/1/front_fr.200.jpg":                   "",
		"https://images.openfoodfacts.org.example.com/x.jpg":                                   "",
		"https://example.com/images/products/1.jpg":                                            "",
		"https://static.openfoodfacts.org/data/dump.jsonl.gz":                                  "",
		"javascript:alert(1)": "",
		"":                    "",
		"  " + img + " ":      img,
	}
	for in, want := range cases {
		if got := ThumbURL(in); got != want {
			t.Errorf("ThumbURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// The URL built from the code follows OFF's folder rule and passes ThumbURL.
func TestFrontImageURL(t *testing.T) {
	cases := map[string]string{
		"8012345678907":  "https://images.openfoodfacts.org/images/products/801/234/567/8907/front_it.3.200.jpg",
		"96385074":       "https://images.openfoodfacts.org/images/products/000/009/638/5074/front_it.3.200.jpg", // padded to 13
		"08012345678907": "https://images.openfoodfacts.org/images/products/080/123/456/78907/front_it.3.200.jpg",
		"ABC/123":        "",
		"":               "",
	}
	for code, want := range cases {
		u := FrontImageURL(code, "it", "3")
		if u != want {
			t.Errorf("FrontImageURL(%q) = %q, want %q", code, u, want)
		}
		if ThumbURL(u) != u {
			t.Errorf("FrontImageURL(%q) produces %q, which ThumbURL discards", code, u)
		}
	}
}
