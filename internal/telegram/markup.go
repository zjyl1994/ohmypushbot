package telegram

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// 使用 Pool 复用 Buffer，减少高频转换时的 GC 压力
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

var (
	// 复用 Markdown 解析器，避免高频场景下的重复构造
	md = goldmark.New()
	// 预编译正则以提高检测性能
	tgFormatRegex = regexp.MustCompile(`\\[.!#\-+=\\*_\[\]()]|\|\|.*?\|\|`)
	// Telegram MarkdownV2 要求的转义映射
	tgEscaper = strings.NewReplacer(
		"_", "\\_", "*", "\\*", "[", "\\[", "]", "\\]", "(", "\\(",
		")", "\\)", "~", "\\~", "`", "\\`", ">", "\\>", "#", "\\#",
		"+", "\\+", "-", "\\-", "=", "\\=", "|", "\\|", "{", "\\{",
		"}", "\\}", ".", "\\.", "!", "\\!",
	)
	codeEscaper = strings.NewReplacer("`", "\\`", "\\", "\\\\")
)

// SmartConvert 是对外的核心入口函数
func SmartConvert(input string) string {
	if input == "" {
		return ""
	}

	// 1. 快速检查：如果已经符合 TG 格式，直接返回
	if tgFormatRegex.MatchString(input) {
		return input
	}

	// 2. 复用全局解析器
	reader := text.NewReader([]byte(input))
	doc := md.Parser().Parse(reader)

	// 3. 从 Pool 获取 Buffer
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	// 4. 遍历并重构
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		switch n.Kind() {
		case ast.KindText:
			if entering {
				t := n.(*ast.Text)
				buf.WriteString(tgEscaper.Replace(string(t.Value(reader.Source()))))
			}

		case ast.KindEmphasis:
			e := n.(*ast.Emphasis)
			mark := "_" // 默认为斜体
			if e.Level == 2 {
				mark = "*" // 加粗
			}
			buf.WriteString(mark)

		case ast.KindCodeSpan:
			buf.WriteString("`")

		case ast.KindLink:
			l := n.(*ast.Link)
			if entering {
				buf.WriteString("[")
			} else {
				buf.WriteString("](")
				buf.WriteString(safeURLEncode(string(l.Destination)))
				buf.WriteString(")")
			}

		case ast.KindFencedCodeBlock, ast.KindCodeBlock:
			if entering {
				// 获取语言标识（如果有）
				lang := ""
				if f, ok := n.(*ast.FencedCodeBlock); ok && f.Info != nil {
					lang = string(f.Info.Value(reader.Source()))
				}
				buf.WriteString("```" + lang + "\n")
				for i := 0; i < n.Lines().Len(); i++ {
					line := n.Lines().At(i)
					buf.WriteString(codeEscaper.Replace(string(line.Value(reader.Source()))))
				}
				buf.WriteString("```\n")
			}
			return ast.WalkSkipChildren, nil

		case ast.KindHeading:
			// 标题在 TG 中不支持，统一转为加粗
			buf.WriteString("*")

		case ast.KindParagraph:
			if !entering && n.NextSibling() != nil {
				buf.WriteString("\n\n")
			}

			// 其他节点保持 Continue 即可
		}
		return ast.WalkContinue, nil
	})

	return buf.String()
}

// safeURLEncode 确保 URL 中的括号被正确处理，防止破坏 TG 的 [text](url) 结构
func safeURLEncode(rawURL string) string {
	u, err := url.Parse(rawURL)
	var target string
	if err != nil {
		target = rawURL
	} else {
		target = u.String()
	}
	// 额外处理括号，这是 TG MarkdownV2 最常见的报错原因
	return strings.NewReplacer("(", "%28", ")", "%29").Replace(target)
}
