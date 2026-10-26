package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/yudi/meet-asr/core-go/modelcatalog"
	"github.com/yudi/meet-asr/core-go/taskmanifest"
)

const vadModelSHA = "9e2449e1087496d8d4caba907f23e0bd3f78d91fa552479bb9c23ac09cbb1fd6"
const vadParameters = "silero-th035-s050-m010-pad150-soft20-hard30-v1"
const vadResults = "transcripts/vad-segments.json"
const vadTimed = "transcripts/transcript-timed.md"

func vadModelPath() (string, error) {
	dir := filepath.Dir(modelcatalog.RuntimeBinary())
	for _, path := range []string{filepath.Join(dir, "silero_vad.onnx"), filepath.Join(dir, "..", "Resources", "silero_vad.onnx")} {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() != 643854 {
			continue
		}
		hash, err := fileSHA256(path)
		if err == nil && hash == vadModelSHA {
			return path, nil
		}
	}
	return "", fmt.Errorf("the verified bundled Silero VAD model is missing; rebuild or reinstall Voxly")
}

// Only a fixed, application-owned scratch WAV is reclaimable. Task locks are
// held by callers; unknown files and links are never deleted to make room.
func cleanVADWork(taskDir string) error {
	dir := filepath.Join(taskDir, ".vad-work")
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe VAD work directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		file, err := os.Lstat(filepath.Join(dir, entry.Name()))
		if err != nil || entry.Name() != "audio.wav" || !file.Mode().IsRegular() {
			return fmt.Errorf("unknown or unsafe VAD work file")
		}
	}
	for _, entry := range entries {
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return os.Remove(dir)
}

func validVADPlan(m *Manifest) error {
	p := m.Chunks.VAD
	if p == nil || !p.Ready || p.Version != 1 || p.ModelSHA256 != vadModelSHA || p.Parameters != vadParameters || p.TotalSamples <= 0 || m.Chunks.Format != "wav" || m.Chunks.Total != len(m.Chunks.Items) {
		return fmt.Errorf("invalid or incomplete VAD plan")
	}
	if _, err := normalizeLocalASRLanguage(p.Language); err != nil {
		return err
	}
	var previous int64
	for i, item := range m.Chunks.Items {
		if item.ID != fmt.Sprintf("part_%03d", i+1) || item.StartSample < previous || item.EndSample <= item.StartSample || item.EndSample > p.TotalSamples || item.EndSample-item.StartSample > 30*16000 {
			return fmt.Errorf("invalid VAD sample range")
		}
		previous = item.EndSample
	}
	return nil
}

func prepareVADChunks(ctx context.Context, taskDir string, m *Manifest, settings *Settings) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.Chunks.VAD == nil {
		return fmt.Errorf("missing VAD configuration")
	}
	if err := cleanVADWork(taskDir); err != nil {
		return err
	}
	work := filepath.Join(taskDir, ".vad-work")
	if err := os.Mkdir(work, 0700); err != nil {
		return err
	}
	defer cleanVADWork(taskDir)
	sourceHash, err := fileSHA256(m.Source.Path)
	if err != nil {
		return err
	}
	if m.Chunks.VAD.Ready && sourceHash != m.Chunks.VAD.SourceSHA256 {
		return fmt.Errorf("source audio changed; the saved VAD plan cannot be reused")
	}
	wav := filepath.Join(work, "audio.wav")
	convert := exec.CommandContext(ctx, ffmpegBinary(), "-nostdin", "-v", "error", "-i", m.Source.Path, "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", "-y", wav)
	if err := convert.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("VAD audio conversion failed: %w", err)
	}
	if !m.Chunks.VAD.Ready {
		model, err := vadModelPath()
		if err != nil {
			return err
		}
		command := exec.CommandContext(ctx, modelcatalog.RuntimeBinary(), "detect-vad", model, wav)
		command.Env = []string{}
		output, err := command.Output()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("VAD detection failed: %w", err)
		}
		var detected struct {
			Rate     int   `json:"sample_rate"`
			Samples  int64 `json:"num_samples"`
			Segments []struct {
				Start int64 `json:"start_sample"`
				End   int64 `json:"end_sample"`
			} `json:"segments"`
		}
		if json.Unmarshal(output, &detected) != nil || detected.Rate != 16000 || detected.Samples <= 0 || len(detected.Segments) > 1000000 {
			return fmt.Errorf("invalid VAD response")
		}
		language, err := normalizeLocalASRLanguage(settings.LocalASRLanguage)
		if err != nil {
			return err
		}
		m.Chunks.VAD = &taskmanifest.VADPlan{Version: 1, ModelSHA256: vadModelSHA, Parameters: vadParameters, Language: language, SourceSHA256: sourceHash, TotalSamples: detected.Samples, Ready: true}
		m.Chunks.Format = "wav"
		m.Chunks.Items = make([]ChunkItem, len(detected.Segments))
		for i, segment := range detected.Segments {
			id := fmt.Sprintf("part_%03d", i+1)
			m.Chunks.Items[i] = ChunkItem{ID: id, AudioPath: filepath.Join(taskDir, "chunks", id+".wav"), TranscriptPath: filepath.Join(taskDir, "transcripts", id+".txt"), State: ChunkPending, StartSample: segment.Start, EndSample: segment.End}
		}
		m.Chunks.Total = len(m.Chunks.Items)
		if err := validVADPlan(m); err != nil {
			return err
		}
		// Commit the complete plan before the first cache file. Resume never
		// re-runs detection for a committed plan, even after cache removal.
		if err := SaveManifest(taskDir, m); err != nil {
			return err
		}
	}
	if err := validVADPlan(m); err != nil {
		return err
	}
	settings.LocalASRLanguage = m.Chunks.VAD.Language
	return materializeVADWave(ctx, taskDir, m, wav)
}

// Locate PCM data without assuming ffmpeg's optional LIST/header chunk sizes.
func pcmData(file *os.File) (int64, int64, error) {
	header := make([]byte, 12)
	if _, err := io.ReadFull(file, header); err != nil {
		return 0, 0, err
	}
	if string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" {
		return 0, 0, fmt.Errorf("invalid PCM WAV")
	}
	formatOK := false
	for {
		chunk := make([]byte, 8)
		if _, err := io.ReadFull(file, chunk); err != nil {
			return 0, 0, err
		}
		size := int64(binary.LittleEndian.Uint32(chunk[4:]))
		switch string(chunk[:4]) {
		case "fmt ":
			if size < 16 || size > 65536 {
				return 0, 0, fmt.Errorf("invalid WAV format")
			}
			data := make([]byte, size)
			if _, err := io.ReadFull(file, data); err != nil {
				return 0, 0, err
			}
			formatOK = binary.LittleEndian.Uint16(data) == 1 && binary.LittleEndian.Uint16(data[2:]) == 1 && binary.LittleEndian.Uint32(data[4:]) == 16000 && binary.LittleEndian.Uint16(data[14:]) == 16
		case "data":
			if !formatOK || size%2 != 0 {
				return 0, 0, fmt.Errorf("expected 16 kHz mono PCM16")
			}
			offset, err := file.Seek(0, io.SeekCurrent)
			return offset, size / 2, err
		default:
			if _, err := file.Seek(size, io.SeekCurrent); err != nil {
				return 0, 0, err
			}
		}
		if size%2 != 0 {
			if _, err := file.Seek(1, io.SeekCurrent); err != nil {
				return 0, 0, err
			}
		}
	}
}

func pcmHeader(samples int64) []byte {
	h := make([]byte, 44)
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+samples*2))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 1)
	binary.LittleEndian.PutUint32(h[24:], 16000)
	binary.LittleEndian.PutUint32(h[28:], 32000)
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(samples*2))
	return h
}

func materializeVADWave(ctx context.Context, taskDir string, m *Manifest, wav string) error {
	if err := validVADPlan(m); err != nil {
		return err
	}
	file, err := os.Open(wav)
	if err != nil {
		return err
	}
	defer file.Close()
	offset, total, err := pcmData(file)
	if err != nil {
		return err
	}
	if total != m.Chunks.VAD.TotalSamples {
		return fmt.Errorf("decoded audio duration differs from saved VAD plan")
	}
	dir := filepath.Join(taskDir, "chunks")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe VAD cache directory")
	}
	for i := range m.Chunks.Items {
		if err := ctx.Err(); err != nil {
			return err
		}
		item := &m.Chunks.Items[i]
		item.AudioPath = filepath.Join(dir, item.ID+".wav")
		item.TranscriptPath = filepath.Join(taskDir, "transcripts", item.ID+".txt")
		if completedTranscriptValid(item) {
			continue
		}
		if info, err := os.Lstat(item.AudioPath); err == nil && !info.Mode().IsRegular() {
			return fmt.Errorf("unsafe VAD cache file")
		}
		length := item.EndSample - item.StartSample
		var out bytes.Buffer
		out.Write(pcmHeader(length))
		if _, err := io.CopyN(&out, io.NewSectionReader(file, offset+item.StartSample*2, length*2), length*2); err != nil {
			return err
		}
		if err := WriteFileAtomically(item.AudioPath, out.Bytes(), 0600); err != nil {
			return err
		}
		hash, err := fileSHA256(item.AudioPath)
		if err != nil {
			return err
		}
		item.AudioSHA256 = hash
		item.SizeBytes = int64(out.Len())
		item.State = ChunkPending
		if err := SaveManifest(taskDir, m); err != nil {
			return err
		}
	}
	return nil
}

func vadArtifactsValid(taskDir string, m *Manifest) bool {
	if m.Chunks.VAD == nil {
		return true
	}
	if validVADPlan(m) != nil {
		return false
	}
	for path, want := range map[string]string{vadResults: m.Chunks.VAD.ResultsSHA256, vadTimed: m.Chunks.VAD.TimedSHA256} {
		if want == "" || !regularCacheInput(taskDir, filepath.Join(taskDir, path)) {
			return false
		}
		got, err := fileSHA256(filepath.Join(taskDir, path))
		if err != nil || got != want {
			return false
		}
	}
	return true
}

func vadTimestamp(sample int64) string {
	ms := sample * 1000 / 16000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

func writeVADResults(taskDir string, m *Manifest) error {
	if m.Chunks.VAD == nil {
		return nil
	}
	if err := validVADPlan(m); err != nil {
		return err
	}
	type row struct {
		ID    string `json:"id"`
		Start int64  `json:"start_ms"`
		End   int64  `json:"end_ms"`
		Text  string `json:"text"`
	}
	rows := make([]row, 0, len(m.Chunks.Items))
	var timed strings.Builder
	for _, item := range m.Chunks.Items {
		expected := filepath.Join(taskDir, "transcripts", item.ID+".txt")
		if item.TranscriptPath != expected || !completedTranscriptValid(&item) || !regularCacheInput(taskDir, expected) {
			return fmt.Errorf("VAD transcript checkpoint is not verified")
		}
		text, err := os.ReadFile(expected)
		if err != nil {
			return err
		}
		value := strings.TrimSpace(string(text))
		rows = append(rows, row{item.ID, item.StartSample * 1000 / 16000, item.EndSample * 1000 / 16000, value})
		fmt.Fprintf(&timed, "[%s–%s] %s\n\n", vadTimestamp(item.StartSample), vadTimestamp(item.EndSample), value)
	}
	if len(rows) == 0 {
		timed.WriteString("未检测到可识别的讲话。\n")
	}
	data, err := json.MarshalIndent(struct {
		Version    int    `json:"schema_version"`
		Rate       int    `json:"sample_rate"`
		Language   string `json:"language"`
		ModelSHA   string `json:"vad_model_sha256"`
		Parameters string `json:"parameters"`
		Segments   []row  `json:"segments"`
	}{1, 16000, m.Chunks.VAD.Language, vadModelSHA, vadParameters, rows}, "", "  ")
	if err != nil {
		return err
	}
	if err := WriteFileAtomically(filepath.Join(taskDir, vadResults), append(data, '\n'), 0600); err != nil {
		return err
	}
	if err := WriteFileAtomically(filepath.Join(taskDir, vadTimed), []byte(timed.String()), 0600); err != nil {
		return err
	}
	m.Chunks.VAD.ResultsSHA256, err = fileSHA256(filepath.Join(taskDir, vadResults))
	if err != nil {
		return err
	}
	m.Chunks.VAD.TimedSHA256, err = fileSHA256(filepath.Join(taskDir, vadTimed))
	return err
}

func mergeTaskTranscript(taskDir string, m *Manifest, output string) error {
	if m.Chunks.VAD == nil {
		return MergeTranscripts(filepath.Join(taskDir, "transcripts"), output)
	}
	if err := validVADPlan(m); err != nil {
		return err
	}
	var text strings.Builder
	for _, item := range m.Chunks.Items {
		if !completedTranscriptValid(&item) {
			return fmt.Errorf("VAD transcript is incomplete")
		}
		data, err := os.ReadFile(filepath.Join(taskDir, "transcripts", item.ID+".txt"))
		if err != nil {
			return err
		}
		value := strings.TrimSpace(string(data))
		if value != "" {
			if text.Len() > 0 {
				text.WriteString("\n\n")
			}
			text.WriteString(value)
		}
	}
	if text.Len() == 0 {
		text.WriteString("未检测到可识别的讲话。")
	}
	if err := WriteFileAtomically(output, []byte(text.String()+"\n"), 0600); err != nil {
		return err
	}
	return writeVADResults(taskDir, m)
}
