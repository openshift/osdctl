package dynatrace

import (
	"errors"
	"io/ioutil"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

func Open(filepath string) (*hclwrite.File, error) {
	content, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	file, diags := hclwrite.ParseConfig(content, filepath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		err := errors.New("an error occurred")
		if err != nil {
			return nil, err
		}
	}
	return file, nil
}

func UpdateDefaultValue(file *hclwrite.File, name string, value string) bool {
	for _, block := range file.Body().Blocks() {
		labels := block.Labels()
		if block.Type() == "module" && len(labels) > 0 && name == labels[0] {
			if block.Body().GetAttribute("source") != nil {
				block.Body().SetAttributeValue("source", cty.StringVal(value))
				return true
			}
		}
	}
	return false
}

func Save(filename string, file *hclwrite.File) error {
	if err := os.WriteFile(filename, file.Bytes(), 0600); err != nil {
		return err
	}
	return nil
}
