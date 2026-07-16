package x

import (
	"regexp"
	"strings"
)

var (
	numberSequence      = regexp.MustCompile(`([a-zA-Z]{0,1})(\d+)([a-zA-Z]{0,1})`)
	firstCamelCaseStart = regexp.MustCompile(`([A-Z]+)([A-Z]?[_a-z\d]+)|$`)
	firstCamelCaseEnd   = regexp.MustCompile(`([\w\W]*?)([_]?[A-Z]+)$`)
)

// RemoveTopNLines 移除cmd返回的前N行数据
func RemoveTopNLines(str string, n int) string {
	strArr := strings.Split(str, "\n")
	var result []string
	for idx, item := range strArr {
		if idx <= n {
			continue
		}
		result = append(result, item)
	}
	return strings.Join(result, "\n")
}

// CmdlineFormatWrap 去除cmd的换行符号
func CmdlineFormatWrap(str string) string {
	str = strings.Replace(str, "\n", "", -1)
	return str
}

// CmdlineFormat 命令行字符串format
func CmdlineFormat(str string) string {
	str = strings.Replace(str, " ", "", -1)
	// 去除换行符
	str = strings.Replace(str, "\n", "", -1)
	return str
}

func Split(str, delimiter string) []string {
	return strings.Split(str, delimiter)
}

func Replace(origin, search, replace string, count ...int) string {
	n := -1
	if len(count) > 0 {
		n = count[0]
	}
	return strings.Replace(origin, search, replace, n)
}

func Equal(a, b string) bool {
	return strings.EqualFold(a, b)
}

func SubStr(str string, start int, length ...int) (substr string) {
	lth := len(str)
	// Simple border checks.
	if start < 0 {
		start = 0
	}
	if start >= lth {
		start = lth
	}
	end := lth
	if len(length) > 0 {
		end = start + length[0]
		if end < start {
			end = lth
		}
	}
	if end > lth {
		end = lth
	}
	return str[start:end]
}

func Join(array []string, sep string) string {
	return strings.Join(array, sep)
}

func UcWords(str string) string {
	return strings.Title(str)
}

func ToUpper(s string) string {
	return strings.ToUpper(s)
}

// CaseCamelLower converts a string to lowerCamelCase.
func CaseCamelLower(s string) string {
	if s == "" {
		return s
	}
	if r := rune(s[0]); r >= 'A' && r <= 'Z' {
		s = strings.ToLower(string(r)) + s[1:]
	}
	return toCamelInitCase(s, false)
}

// Converts a string to CamelCase
func toCamelInitCase(s string, initCase bool) string {
	s = addWordBoundariesToNumbers(s)
	s = strings.Trim(s, " ")
	n := ""
	capNext := initCase
	for _, v := range s {
		if v >= 'A' && v <= 'Z' {
			n += string(v)
		}
		if v >= '0' && v <= '9' {
			n += string(v)
		}
		if v >= 'a' && v <= 'z' {
			if capNext {
				n += strings.ToUpper(string(v))
			} else {
				n += string(v)
			}
		}
		if v == '_' || v == ' ' || v == '-' || v == '.' {
			capNext = true
		} else {
			capNext = false
		}
	}
	return n
}

func addWordBoundariesToNumbers(s string) string {
	r := numberSequence.ReplaceAllFunc([]byte(s), func(bytes []byte) []byte {
		var result []byte
		match := numberSequence.FindSubmatch(bytes)
		if len(match[1]) > 0 {
			result = append(result, match[1]...)
			result = append(result, []byte(" ")...)
		}
		result = append(result, match[2]...)
		if len(match[3]) > 0 {
			result = append(result, []byte(" ")...)
			result = append(result, match[3]...)
		}
		return result
	})
	return string(r)
}
