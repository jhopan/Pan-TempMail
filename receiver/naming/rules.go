// Package naming generates inbox local-parts from configurable rules.
// Rules are stored as JSON in the settings table (key "naming_rules")
// and can be changed at runtime via the admin API.
package naming

import (
	crypto_rand "crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

// Rules define how a random local-part is built.
type Rules struct {
	// Template tokens: {first} {second} {sep} {num} {rand:N} {ts}
	Template       string   `json:"template"`
	Separator      string   `json:"separator"`
	FirstWords     []string `json:"first_words"`
	SecondWords    []string `json:"second_words"`
	NumMin         int      `json:"num_min"`
	NumMax         int      `json:"num_max"`
	NumProbability int      `json:"num_probability"` // percent 0-100
}

// Default returns the built-in rules (human-like Indonesian word lists).
func Default() Rules {
	return Rules{
		Template:  "{first}{sep}{second}{num}",
		Separator: "",
		FirstWords: []string{
			"langit", "senja", "pagi", "malam", "kopi", "hujan", "bulan", "bintang", "angin", "awan",
			"nusa", "rasa", "jalan", "teman", "cerita", "warna", "nadi", "cahya", "putra", "putri",
			"bagas", "adit", "arya", "dimas", "fajar", "rizky", "galih", "bayu", "bima", "nanda",
			"mega", "surya", "gunung", "lembah", "samudra", "rimba", "danau", "pelangi", "embun", "pelita",
			"elang", "macan", "rajawali", "garuda", "merak", "kancil", "naga", "kuda", "merpati", "kenari",
			"melati", "mawar", "anggrek", "teratai", "cemara", "pinus", "jati", "mangga", "durian", "salak",
			"ananta", "arjuna", "bisma", "dewa", "dewi", "galuh", "kresna", "sinta", "wibawa", "wisesa",
			"agung", "abimanyu", "cakra", "prabu", "ratna", "sakti", "satria", "wira", "widya", "nirmala",
		},
		SecondWords: []string{
			"biru", "jingga", "ungu", "merah", "kuning", "hijau", "putih", "emas", "perak", "nila",
			"manis", "tenang", "indah", "muda", "asri", "jaya", "lucu", "syahdu", "harum", "utama",
			"cerdas", "cepat", "lembut", "hangat", "murni", "setia", "mulia", "ramah", "santun", "gembira",
			"perkasa", "sabar", "ikhlas", "tabah", "cermat", "cekatan", "lapang", "damai", "kencang", "lembayung",
			"laut", "hutan", "cerah", "kecil", "besar", "rimbun", "sawah", "pulau", "tebing", "bukit",
			"aji", "ayu", "sari", "nugraha", "permata", "lestari", "mahesa", "karya", "puspa", "bakti",
			"citra", "daya", "dharma", "gita", "jiwa", "karsa", "kencana", "loka", "mustika", "bentala",
			"gerhana", "kirana", "mahkota", "mentari", "nusantara", "purnama", "sejati", "rasa", "cinta", "harapan",
		},
		NumMin:         10,
		NumMax:         99,
		NumProbability: 80,
	}
}

// Parse decodes rules from JSON, falling back to defaults for empty fields.
func Parse(data string) (Rules, error) {
	r := Default()
	if strings.TrimSpace(data) == "" {
		return r, nil
	}
	if err := json.Unmarshal([]byte(data), &r); err != nil {
		return Default(), fmt.Errorf("parse naming rules: %w", err)
	}
	if r.Template == "" {
		r.Template = "{first}{sep}{second}{num}"
	}
	if len(r.FirstWords) == 0 || len(r.SecondWords) == 0 {
		return Default(), fmt.Errorf("naming rules need non-empty first_words and second_words")
	}
	if r.NumMax < r.NumMin {
		r.NumMax = r.NumMin
	}
	if r.NumProbability < 0 {
		r.NumProbability = 0
	}
	if r.NumProbability > 100 {
		r.NumProbability = 100
	}
	return r, nil
}

// randInt returns a cryptographically random int in [min, max].
func randInt(min, max int) int {
	if max <= min {
		return min
	}
	n, err := crypto_rand.Int(crypto_rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return min
	}
	return min + int(n.Int64())
}

func pick(words []string) string {
	return words[randInt(0, len(words)-1)]
}

// Render builds one local-part from the rules.
func (r Rules) Render() string {
	out := r.Template
	out = strings.ReplaceAll(out, "{first}", pick(r.FirstWords))
	out = strings.ReplaceAll(out, "{second}", pick(r.SecondWords))
	out = strings.ReplaceAll(out, "{sep}", r.Separator)

	num := ""
	if r.NumProbability > 0 && randInt(1, 100) <= r.NumProbability {
		num = strconv.Itoa(randInt(r.NumMin, r.NumMax))
	}
	out = strings.ReplaceAll(out, "{num}", num)

	// {rand:N} → N random lowercase alphanumeric chars
	for {
		i := strings.Index(out, "{rand:")
		if i < 0 {
			break
		}
		j := strings.Index(out[i:], "}")
		if j < 0 {
			break
		}
		n, _ := strconv.Atoi(out[i+6 : i+j])
		if n < 1 {
			n = 6
		}
		if n > 32 {
			n = 32
		}
		const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
		sb := make([]byte, n)
		for k := range sb {
			sb[k] = charset[randInt(0, len(charset)-1)]
		}
		out = out[:i] + string(sb) + out[i+j+1:]
	}

	out = strings.ReplaceAll(out, "{ts}", strconv.FormatInt(time.Now().Unix(), 10))
	return strings.ToLower(out)
}

// GenerateUnique renders addresses until exists() returns false.
// Fallback appends a timestamp suffix after 50 collisions.
func (r Rules) GenerateUnique(exists func(addr string) bool) string {
	for i := 0; i < 50; i++ {
		addr := r.Render()
		if addr != "" && !exists(addr) {
			return addr
		}
	}
	for i := 0; i < 10; i++ {
		addr := r.Render() + strconv.FormatInt(time.Now().UnixMilli()%100000, 10)
		if !exists(addr) {
			return addr
		}
	}
	// Last resort: guaranteed-unique random suffix
	return r.Render() + strconv.FormatInt(time.Now().UnixNano()%1000000000, 10)
}
