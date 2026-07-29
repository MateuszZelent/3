// Package template expands parameter expressions embedded in MX3 templates.
package template

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Options struct {
	Flat bool
}

type expression struct {
	prefix, suffix, format, original string
	values                           []string
	numeric                          bool
}

var expressionPattern = regexp.MustCompile(`"\{(.*?)\}"`)
var formatPattern = regexp.MustCompile(`^%[-+]?[0-9]*(\.[0-9]*)?[fs]$`)

func Generate(filename string, options Options) ([]string, error) {
	absolute, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}
	contents, err := os.ReadFile(absolute)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}
	expressions, err := parseExpressions(string(contents))
	if err != nil {
		return nil, err
	}
	return generate(filepath.Dir(absolute), string(contents), expressions, options.Flat)
}

func parseExpressions(contents string) ([]expression, error) {
	matches := expressionPattern.FindAllStringSubmatch(contents, -1)
	if len(matches) == 0 {
		return nil, errors.New("no template expressions found")
	}
	result := make([]expression, 0, len(matches))
	for _, match := range matches {
		fields := map[string]string{}
		for _, part := range strings.Split(match[1], ";") {
			key, value, ok := strings.Cut(part, "=")
			if !ok {
				return nil, fmt.Errorf("invalid expression part %q", part)
			}
			fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
		fields["original"] = match[1]
		expr, err := parseExpression(fields)
		if err != nil {
			return nil, err
		}
		result = append(result, expr)
	}
	return result, nil
}

func parseExpression(fields map[string]string) (expression, error) {
	valid := map[string]bool{"prefix": true, "suffix": true, "format": true, "array": true, "start": true, "end": true, "step": true, "count": true, "original": true}
	for key := range fields {
		if !valid[key] {
			return expression{}, fmt.Errorf("invalid field %q", key)
		}
	}
	format := fields["format"]
	if format == "" {
		format = "%v"
	}
	if format != "%v" && !formatPattern.MatchString(format) {
		return expression{}, fmt.Errorf("invalid format %q", format)
	}
	values, numeric, err := expressionValues(fields)
	if err != nil {
		return expression{}, err
	}
	if format == "%s" && numeric {
		numeric = false
	}
	if strings.HasSuffix(format, "f") && !numeric {
		return expression{}, fmt.Errorf("numeric format %q used with string array", format)
	}
	return expression{prefix: fields["prefix"], suffix: fields["suffix"], format: format, original: fields["original"], values: values, numeric: numeric}, nil
}

func expressionValues(fields map[string]string) ([]string, bool, error) {
	if raw, ok := fields["array"]; ok {
		for _, conflict := range []string{"start", "end", "step", "count"} {
			if _, exists := fields[conflict]; exists {
				return nil, false, fmt.Errorf("%s cannot be combined with array", conflict)
			}
		}
		raw = strings.TrimSpace(strings.Trim(raw, "[]"))
		if raw == "" {
			return nil, false, errors.New("array cannot be empty")
		}
		parts := strings.Split(raw, ",")
		numeric := true
		for index := range parts {
			parts[index] = strings.Trim(strings.TrimSpace(parts[index]), "\"'")
			if _, err := strconv.ParseFloat(parts[index], 64); err != nil {
				numeric = false
			}
		}
		return parts, numeric, nil
	}
	startRaw, hasStart := fields["start"]
	endRaw, hasEnd := fields["end"]
	if !hasStart || !hasEnd {
		return nil, false, errors.New("start and end are required without array")
	}
	start, err := strconv.ParseFloat(startRaw, 64)
	if err != nil {
		return nil, false, fmt.Errorf("invalid start: %w", err)
	}
	end, err := strconv.ParseFloat(endRaw, 64)
	if err != nil {
		return nil, false, fmt.Errorf("invalid end: %w", err)
	}
	if start > end {
		return nil, false, errors.New("start must not exceed end")
	}
	var numbers []float64
	if countRaw, ok := fields["count"]; ok {
		if _, conflict := fields["step"]; conflict {
			return nil, false, errors.New("step and count are mutually exclusive")
		}
		count, err := strconv.Atoi(countRaw)
		if err != nil || count <= 0 {
			return nil, false, errors.New("count must be positive")
		}
		if count > 1000 {
			return nil, false, errors.New("range exceeds 1000 elements")
		}
		if count == 1 {
			numbers = []float64{start}
		} else {
			for i := 0; i < count; i++ {
				numbers = append(numbers, start+float64(i)*(end-start)/float64(count-1))
			}
		}
	} else {
		stepRaw, ok := fields["step"]
		if !ok {
			return nil, false, errors.New("step or count is required")
		}
		step, err := strconv.ParseFloat(stepRaw, 64)
		if err != nil || step <= 0 {
			return nil, false, errors.New("step must be positive")
		}
		for value := start; value <= end; value += step {
			if len(numbers) >= 1000 {
				return nil, false, errors.New("range exceeds 1000 elements")
			}
			numbers = append(numbers, value)
		}
	}
	values := make([]string, len(numbers))
	for i, value := range numbers {
		values[i] = fmt.Sprint(value)
	}
	return values, true, nil
}

func generate(parent, contents string, expressions []expression, flat bool) ([]string, error) {
	count := 1
	for _, expr := range expressions {
		count *= len(expr.values)
	}
	indices := make([]int, len(expressions))
	files := make([]string, 0, count)
	for combination := 0; combination < count; combination++ {
		parts := make([]string, len(expressions))
		output := contents
		for i, expr := range expressions {
			value := expr.values[indices[i]]
			formatted := value
			if expr.format != "%v" {
				if expr.numeric {
					number, _ := strconv.ParseFloat(value, 64)
					formatted = fmt.Sprintf(expr.format, number)
				} else {
					formatted = fmt.Sprintf(expr.format, value)
				}
			}
			parts[i] = expr.prefix + formatted + expr.suffix
			output = strings.Replace(output, `"{`+expr.original+`}"`, value, 1)
		}
		relative := filepath.Join(parts...)
		if flat {
			relative = strings.Join(parts, "")
		}
		filename := filepath.Join(parent, relative+".mx3")
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filename, []byte(output), 0o644); err != nil {
			return nil, err
		}
		files = append(files, filename)
		for i := len(indices) - 1; i >= 0; i-- {
			indices[i]++
			if indices[i] < len(expressions[i].values) {
				break
			}
			indices[i] = 0
		}
	}
	return files, nil
}
