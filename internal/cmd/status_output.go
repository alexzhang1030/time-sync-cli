package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/alexzhang1030/time-sync-cli/internal/status"
)

const defaultStatusOutputWidth = 76

func renderStatusOutput(report *status.Report, format string, terminal, color bool, width int) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "auto", "":
		if terminal {
			return report.FancySummary(status.RenderOptions{Color: color, Width: width}), nil
		}
		return report.Summary(), nil
	case "fancy":
		return report.FancySummary(status.RenderOptions{Color: color, Width: width}), nil
	case "plain":
		return report.Summary(), nil
	case "json":
		output, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return "", fmt.Errorf("encode status JSON: %w", err)
		}
		return string(output) + "\n", nil
	default:
		return "", fmt.Errorf("invalid --output %q: choose auto, fancy, plain, or json", format)
	}
}

func writerIsTerminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func writerSupportsColor(writer io.Writer) bool {
	return writerSupportsFancy(writer) && os.Getenv("NO_COLOR") == ""
}

func writerSupportsFancy(writer io.Writer) bool {
	return writerIsTerminal(writer) && os.Getenv("TERM") != "dumb"
}

func statusOutputWidth() int {
	width, err := strconv.Atoi(os.Getenv("COLUMNS"))
	if err != nil || width < 56 {
		return defaultStatusOutputWidth
	}
	if width > 96 {
		return 96
	}
	return width
}
