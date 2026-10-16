package corpus

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadIDList reads a JSON file of the form {"rulings": ["N123456", ...]},
// such as data/textile_rulings.json (D51).
func LoadIDList(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("corpus: read %s: %w", path, err)
	}
	var f struct {
		Rulings []string `json:"rulings"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("corpus: decode %s: %w", path, err)
	}
	if len(f.Rulings) == 0 {
		return nil, fmt.Errorf("corpus: %s lists no rulings", path)
	}
	return f.Rulings, nil
}
