// Package pdf — конвертация DOCX → PDF через LibreOffice headless.
//
// Многопользовательская работа:
//   - семафор ограничивает число одновременных конвертаций (LibreOffice тяжёлый);
//   - у каждой конвертации свой профиль (-env:UserInstallation) — два LibreOffice
//     с общим профилем блокируют друг друга;
//   - таймаут + убийство всей группы процессов (soffice.bin — дочерний процесс).
package pdf

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const ConvertTimeout = 120 * time.Second

// ProfilePrefix — префикс временных профилей LibreOffice в os.TempDir().
const ProfilePrefix = "lo_profile_"

type Converter struct {
	binary string
	sem    chan struct{}
}

func New(binary string, concurrency int) *Converter {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Converter{binary: binary, sem: make(chan struct{}, concurrency)}
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Convert конвертирует DOCX и возвращает путь к PDF рядом с исходником.
func (c *Converter) Convert(ctx context.Context, docxPath string) (string, error) {
	outDir := filepath.Dir(docxPath)
	profile := filepath.Join(os.TempDir(), ProfilePrefix+randHex(16))
	defer os.RemoveAll(profile)

	select {
	case c.sem <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-c.sem }()

	cmd := exec.Command(c.binary, "--headless", "--norestore",
		"-env:UserInstallation=file://"+profile,
		"--convert-to", "pdf", "--outdir", outDir, docxPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// своя группа процессов: libreoffice — обёртка, работу делает дочерний soffice.bin
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("не удалось запустить LibreOffice (%s): %w", c.binary, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	timer := time.NewTimer(ConvertTimeout)
	defer timer.Stop()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-timer.C:
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return "", fmt.Errorf("LibreOffice не ответил за %d с — конвертация прервана", int(ConvertTimeout.Seconds()))
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return "", ctx.Err()
	}

	if waitErr != nil {
		var ee *exec.ExitError
		code := -1
		if errors.As(waitErr, &ee) {
			code = ee.ExitCode()
		}
		return "", fmt.Errorf("LibreOffice вернул код %d.\nstderr: %s", code, strings.TrimSpace(stderr.String()))
	}

	pdfPath := strings.TrimSuffix(docxPath, filepath.Ext(docxPath)) + ".pdf"
	if _, err := os.Stat(pdfPath); err != nil {
		// LibreOffice часто завершается с кодом 0, даже если не смог открыть файл
		return "", fmt.Errorf("PDF не создан: %s\nstdout: %s\nstderr: %s",
			pdfPath, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}
	return pdfPath, nil
}
