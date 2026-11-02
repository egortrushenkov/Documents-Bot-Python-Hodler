package docgen

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/beevik/etree"

	"hodler-doc-bot/internal/config"
	"hodler-doc-bot/internal/util"
)

const (
	nsW  = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	nsWP = "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
)

// ErrTemplateNotFound — нет файла шаблона.
var ErrTemplateNotFound = errors.New("шаблон не найден")

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func isW(e *etree.Element, tag string) bool {
	return e.Tag == tag && (e.NamespaceURI() == nsW || e.Space == "w")
}

// ─── Работа с ранами (w:r) ────────────────────────────────────────────────────

func hasDrawing(r *etree.Element) bool {
	var walk func(e *etree.Element) bool
	walk = func(e *etree.Element) bool {
		for _, c := range e.ChildElements() {
			if (c.Tag == "anchor" || c.Tag == "inline") && (c.NamespaceURI() == nsWP || c.Space == "wp") {
				return true
			}
			if walk(c) {
				return true
			}
		}
		return false
	}
	return walk(r)
}

func isTextChild(c *etree.Element) bool {
	if c.NamespaceURI() != nsW && c.Space != "w" {
		return false
	}
	switch c.Tag {
	case "t", "tab", "br", "cr", "noBreakHyphen", "ptab":
		return true
	}
	return false
}

// runText — текст рана (как run.text в python-docx).
func runText(r *etree.Element) string {
	var b strings.Builder
	for _, c := range r.ChildElements() {
		if !isTextChild(c) {
			continue
		}
		switch c.Tag {
		case "t":
			b.WriteString(c.Text())
		case "tab", "ptab":
			b.WriteByte('\t')
		case "br", "cr":
			b.WriteByte('\n')
		case "noBreakHyphen":
			b.WriteByte('-')
		}
	}
	return b.String()
}

// setRunText заменяет текстовое содержимое рана, сохраняя форматирование (w:rPr)
// и прочие нетекстовые элементы.
func setRunText(r *etree.Element, text string) {
	for _, c := range r.ChildElements() {
		if isTextChild(c) {
			r.RemoveChild(c)
		}
	}
	prefix := r.Space
	if prefix == "" {
		prefix = "w"
	}
	var buf strings.Builder
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		t := r.CreateElement(prefix + ":t")
		t.CreateAttr("xml:space", "preserve")
		t.SetText(buf.String())
		buf.Reset()
	}
	for _, ch := range text {
		switch ch {
		case '\t':
			flush()
			r.CreateElement(prefix + ":tab")
		case '\n', '\r':
			flush()
			r.CreateElement(prefix + ":br")
		default:
			buf.WriteRune(ch)
		}
	}
	flush()
}

// paragraphRuns — прямые раны абзаца (как paragraph.runs в python-docx).
func paragraphRuns(p *etree.Element) []*etree.Element {
	var out []*etree.Element
	for _, c := range p.ChildElements() {
		if isW(c, "r") {
			out = append(out, c)
		}
	}
	return out
}

func paragraphText(p *etree.Element) string {
	var b strings.Builder
	for _, r := range paragraphRuns(p) {
		b.WriteString(runText(r))
	}
	return b.String()
}

func findAll(root *etree.Element, tag string) []*etree.Element {
	var out []*etree.Element
	var walk func(e *etree.Element)
	walk = func(e *etree.Element) {
		for _, c := range e.ChildElements() {
			if isW(c, tag) {
				out = append(out, c)
			}
			walk(c)
		}
	}
	walk(root)
	return out
}

func directChildren(e *etree.Element, tag string) []*etree.Element {
	var out []*etree.Element
	for _, c := range e.ChildElements() {
		if isW(c, tag) {
			out = append(out, c)
		}
	}
	return out
}

// ─── Замена плейсхолдеров ─────────────────────────────────────────────────────

func replaceAll(s string, pairs []Pair) string {
	for _, p := range pairs {
		if strings.Contains(s, p.Key) {
			s = strings.ReplaceAll(s, p.Key, p.Value)
		}
	}
	return s
}

// replaceInParagraph учитывает, что Word может разбить плейсхолдер на несколько
// ранов (`{{OP_`, `KIO`, `}}`): соседние текстовые раны склеиваются. Раны с
// картинками (подпись/печать) разделяют группы — текст через них не «прыгает».
func replaceInParagraph(p *etree.Element, pairs []Pair) {
	runs := paragraphRuns(p)
	if len(runs) == 0 || !strings.Contains(paragraphText(p), "{{") {
		return
	}
	flush := func(group []*etree.Element) {
		if len(group) == 0 {
			return
		}
		var b strings.Builder
		for _, r := range group {
			b.WriteString(runText(r))
		}
		full := b.String()
		if !strings.Contains(full, "{{") {
			return
		}
		repl := replaceAll(full, pairs)
		if repl == full {
			return
		}
		setRunText(group[0], repl)
		for _, r := range group[1:] {
			setRunText(r, "")
		}
	}
	var group []*etree.Element
	for _, r := range runs {
		if hasDrawing(r) {
			flush(group)
			group = nil
		} else {
			group = append(group, r)
		}
	}
	flush(group)
}

func replaceInTree(root *etree.Element, pairs []Pair) {
	for _, p := range findAll(root, "p") {
		replaceInParagraph(p, pairs)
	}
}

// ─── Таблица транзакций ───────────────────────────────────────────────────────

func rowText(tr *etree.Element) string {
	var b strings.Builder
	for _, p := range findAll(tr, "p") {
		b.WriteString(paragraphText(p))
	}
	return b.String()
}

// mergeRowRuns склеивает раны в каждом абзаце строки — плейсхолдеры становятся целыми.
func mergeRowRuns(tr *etree.Element) {
	for _, p := range findAll(tr, "p") {
		var runs []*etree.Element
		for _, r := range paragraphRuns(p) {
			if !hasDrawing(r) {
				runs = append(runs, r)
			}
		}
		if len(runs) == 0 {
			continue
		}
		var b strings.Builder
		for _, r := range runs {
			b.WriteString(runText(r))
		}
		if full := b.String(); full != "" {
			setRunText(runs[0], full)
			for _, r := range runs[1:] {
				setRunText(r, "")
			}
		}
	}
}

func fillTxRow(tr *etree.Element, tx ComputedTx) {
	va := util.FormatNumber(tx.VAAmount)
	fiat := util.FormatNumber(util.Num(tx.FiatAmount))
	pairs := []Pair{
		{"{{VA_AMOUNT}}", va},
		{"{{VA_AMOUNT_SHORT}}", va},
		{"{{FIAT_AMOUNT}}", fiat},
		{"{{FIAT_AMOUNT_SHORT}}", fiat},
		{"{{TX_HASH}}", tx.TxHash},
	}
	for _, p := range findAll(tr, "p") {
		for _, r := range paragraphRuns(p) {
			if hasDrawing(r) {
				continue
			}
			t := runText(r)
			if t == "" {
				continue
			}
			if n := replaceAll(t, pairs); n != t {
				setRunText(r, n)
			}
		}
	}
}

// expandTransactions находит строку с {{TX_HASH}} и клонирует её по числу
// транзакций. Строка «Итого» после блока остаётся для сводных значений.
func expandTransactions(body *etree.Element, txs []ComputedTx) {
	for _, tbl := range findAll(body, "tbl") {
		rows := directChildren(tbl, "tr")
		dataIdx := -1
		for i, tr := range rows {
			if strings.Contains(rowText(tr), "{{TX_HASH}}") {
				dataIdx = i
				break
			}
		}
		if dataIdx < 0 {
			continue
		}
		dataRow := rows[dataIdx]
		mergeRowRuns(dataRow)

		parent := dataRow.Parent()
		last := dataRow
		for range len(txs) - 1 {
			clone := dataRow.Copy()
			parent.InsertChildAt(last.Index()+1, clone)
			last = clone
		}
		rows = directChildren(tbl, "tr")
		for j, tx := range txs {
			fillTxRow(rows[dataIdx+j], tx)
		}
		if total := dataIdx + len(txs); total < len(rows) {
			mergeRowRuns(rows[total])
		}
		return // таблица транзакций одна
	}
}

// ─── Чтение/запись пакета DOCX ────────────────────────────────────────────────

type docxPackage struct {
	files []*zip.File
	data  map[string][]byte
}

func openDocx(path string) (*docxPackage, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	pkg := &docxPackage{data: map[string][]byte{}}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		fh := f.FileHeader
		pkg.files = append(pkg.files, &zip.File{FileHeader: fh})
		pkg.data[f.Name] = b
	}
	if _, ok := pkg.data["word/document.xml"]; !ok {
		return nil, fmt.Errorf("в файле нет word/document.xml — это не DOCX")
	}
	return pkg, nil
}

var partRe = regexp.MustCompile(`^word/(document|header\d*|footer\d*)\.xml$`)

// textParts — части документа с текстом: тело, колонтитулы.
func (p *docxPackage) textParts() []string {
	var out []string
	for _, f := range p.files {
		if partRe.MatchString(f.Name) {
			out = append(out, f.Name)
		}
	}
	return out
}

func (p *docxPackage) save(path string) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range p.files {
		h := &zip.FileHeader{Name: f.Name, Method: zip.Deflate, Modified: f.Modified}
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		if _, err := w.Write(p.data[f.Name]); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func parseXML(b []byte) (*etree.Document, error) {
	doc := etree.NewDocument()
	doc.ReadSettings.PreserveCData = true
	if err := doc.ReadFromBytes(b); err != nil {
		return nil, err
	}
	return doc, nil
}

// ScanPlaceholders возвращает все {{...}} шаблона (склеивая раны, чтобы найти
// и разбитые на части), в порядке появления, без повторов.
func ScanPlaceholders(path string) ([]string, error) {
	pkg, err := openDocx(path)
	if err != nil {
		return nil, err
	}
	var texts []string
	for _, name := range pkg.textParts() {
		doc, err := parseXML(pkg.data[name])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		for _, p := range findAll(&doc.Element, "p") {
			texts = append(texts, paragraphText(p))
		}
	}
	re := regexp.MustCompile(`\{\{[A-Za-z0-9_]+\}\}`)
	seen := map[string]bool{}
	var out []string
	for _, m := range re.FindAllString(strings.Join(texts, "\n"), -1) {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out, nil
}

// Generate создаёт DOCX по шаблону и возвращает путь к файлу в outputDir.
func Generate(templatesDir, outputDir string, d *Draft, op, client, clientCustom map[string]string) (string, error) {
	actType := d.GetOr("act_type", "sell")
	tplName, ok := config.ActTemplateFiles[actType]
	if !ok {
		tplName = config.ActTemplateFiles["sell"]
	}
	tplPath := filepath.Join(templatesDir, tplName)
	if _, err := os.Stat(tplPath); err != nil {
		return "", fmt.Errorf("%w: %s\nПоложите файл %s в папку %s/", ErrTemplateNotFound, tplPath, tplName, templatesDir)
	}

	// Транзакции-части с рассчитанными рублями; «Итого» по ВА = сумма частей
	txs := ComputeTransactions(d.Txs, d.Get("va_amount"), d.Get("fiat_amount"), d.Get("tx_hash"))
	vaTotal := 0.0
	for _, t := range txs {
		vaTotal += util.ToFloat(t.VAAmount)
	}
	totals := &Draft{Fields: map[string]string{}, Txs: d.Txs}
	for k, v := range d.Fields {
		totals.Fields[k] = v
	}
	totals.Fields["va_amount"] = util.Num(vaTotal)
	totals.Fields["tx_hash"] = "" // хэши подставляются построчно

	direction := map[string]string{"sell": "buy", "buy": "sell", "invoice": "invoice_buy"}[actType]
	if direction == "" {
		direction = actType
	}
	prefix := "act_" + direction
	if actType == "invoice" {
		prefix = "invoice"
	}
	clShort := util.SafeNamePart(client["short_name"], 10, "CL")
	outName := fmt.Sprintf("%s_%s-%s_%s.docx", prefix,
		util.SafeNamePart(d.GetOr("deal_number", "0"), 30, "0"), clShort,
		strings.ReplaceAll(d.Get("deal_date"), ".", ""))
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}
	outPath := filepath.Join(outputDir, outName)

	pkg, err := openDocx(tplPath)
	if err != nil {
		return "", fmt.Errorf("шаблон %s не читается: %w", tplName, err)
	}
	pairs := BuildReplacements(totals, op, client, clientCustom)
	for _, name := range pkg.textParts() {
		doc, err := parseXML(pkg.data[name])
		if err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
		if name == "word/document.xml" {
			// 1) построчно заполняем таблицу транзакций (до общей замены)
			expandTransactions(&doc.Element, txs)
		}
		// 2) общая замена остальных плейсхолдеров (сводные суммы, реквизиты)
		replaceInTree(&doc.Element, pairs)
		b, err := doc.WriteToBytes()
		if err != nil {
			return "", err
		}
		pkg.data[name] = b
	}
	if err := pkg.save(outPath); err != nil {
		return "", err
	}
	return outPath, nil
}
