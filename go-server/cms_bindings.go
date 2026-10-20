package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Parse only literal nginx server blocks. Never execute directives, expand variables or follow includes.
func nginxTokens(data string) []string {
	tokens := []string{}
	var word strings.Builder
	quote := rune(0)
	comment, escaped := false, false
	flush := func() {
		if word.Len() > 0 {
			tokens = append(tokens, word.String())
			word.Reset()
		}
	}
	for _, ch := range data {
		if comment {
			if ch == '\n' {
				comment = false
			}
			continue
		}
		if escaped {
			word.WriteRune(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			} else {
				word.WriteRune(ch)
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		switch ch {
		case '#':
			flush()
			comment = true
		case '{', '}', ';':
			flush()
			tokens = append(tokens, string(ch))
		case ' ', '\t', '\r', '\n':
			flush()
		default:
			word.WriteRune(ch)
		}
	}
	flush()
	return tokens
}
func addCMSBindings(configDirs []string, candidates []M) []string {
	warnings := []string{}
	count, bytes := 0, int64(0)
	for _, dir := range configDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				warnings = append(warnings, "无法读取站点绑定目录："+dir)
			}
			continue
		}
		for _, entry := range entries {
			if count >= 128 || bytes >= 4*1024*1024 {
				return append(warnings, "站点绑定扫描已达到限制，未扫描的站点可手动输入域名检测。")
			}
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".conf") {
				continue
			}
			file := filepath.Join(dir, entry.Name())
			info, e := os.Stat(file)
			if e != nil || info.Size() > 1024*1024 {
				continue
			}
			count++
			bytes += info.Size()
			data, e := os.ReadFile(file)
			if e != nil {
				continue
			}
			tokens := nginxTokens(string(data))
			for i := 0; i+1 < len(tokens); i++ {
				if tokens[i] != "server" || tokens[i+1] != "{" {
					continue
				}
				depth := 1
				directives := [][]string{}
				directive := []string{}
				j := i + 2
				for ; j < len(tokens) && depth > 0; j++ {
					token := tokens[j]
					switch token {
					case "{":
						depth++
						directive = nil
					case "}":
						depth--
						directive = nil
					case ";":
						if depth == 1 && len(directive) > 0 {
							directives = append(directives, directive)
						}
						directive = nil
					default:
						if depth == 1 {
							directive = append(directive, token)
						}
					}
				}
				i = j - 1
				if depth != 0 {
					continue
				}
				root := ""
				names := []string{}
				listeners := [][]string{}
				for _, d := range directives {
					if len(d) < 2 {
						continue
					}
					switch d[0] {
					case "root":
						root = d[1]
					case "server_name":
						names = append(names, d[1:]...)
					case "listen":
						listeners = append(listeners, d[1:])
					}
				}
				if !filepath.IsAbs(root) || strings.Contains(root, "$") {
					continue
				}
				realRoot, e := filepath.EvalSymlinks(root)
				if e != nil {
					continue
				}
				if len(listeners) == 0 {
					listeners = [][]string{{"80"}}
				}
				for _, candidate := range candidates {
					siteRoot := str(candidate["root"])
					if !withinDirectory(realRoot, siteRoot) {
						continue
					}
					rel, e := filepath.Rel(realRoot, siteRoot)
					if e != nil {
						continue
					}
					suffix := ""
					if rel != "." {
						suffix = "/" + filepath.ToSlash(rel)
					}
					previous := candidate["addresses"]
					candidate["addresses"] = []M{}
					for _, listener := range listeners {
						value := listener[0]
						port := value
						if strings.Contains(value, ":") {
							_, port, e = net.SplitHostPort(value)
							if e != nil {
								continue
							}
						}
						number, e := strconv.Atoi(port)
						if e != nil || number < 1 || number > 65535 {
							continue
						}
						scheme := "http"
						if has(listener, "ssl") || number == 443 {
							scheme = "https"
						}
						if has(listener, "quic") {
							continue
						}
						for _, name := range names {
							if strings.ContainsAny(name, "$*~/:\\") || name == "_" {
								continue
							}
							host := name
							if !(scheme == "http" && number == 80 || scheme == "https" && number == 443) {
								host = net.JoinHostPort(name, port)
							}
							addCMSAddress(candidate, fmt.Sprintf("%s://%s%s", scheme, host, suffix), "宝塔 Nginx 站点绑定")
						}
					}
					if old, ok := previous.([]M); ok {
						for _, address := range old {
							addCMSAddress(candidate, str(address["base"]), str(address["source"]))
						}
					}
					addresses, _ := candidate["addresses"].([]M)
					if len(addresses) > 0 {
						candidate["base"] = addresses[0]["base"]
					}
				}
			}
		}
	}
	return warnings
}
