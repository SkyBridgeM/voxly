import { open } from "@tauri-apps/plugin-dialog";

interface FilePickerProps {
  audioPath: string | null;
  onFileSelected: (path: string) => void;
  disabled: boolean;
  labels: {
    chooseFile: string;
    noFileSelected: string;
    audioFile: string;
  };
}

export function FilePicker({
  audioPath,
  onFileSelected,
  disabled,
  labels,
}: FilePickerProps) {
  const pickFile = async () => {
    const selected = await open({
      multiple: false,
      filters: [
        {
          name: "Audio",
          extensions: ["mp3", "wav", "m4a"],
        },
      ],
    });
    if (selected && typeof selected === "string") {
      onFileSelected(selected);
    }
  };

  const getFileName = (path: string) => {
    return path.split("/").pop() || path.split("\\").pop() || path;
  };

  return (
    <div className="file-row">
      <div className="file-visual" aria-hidden="true">
        <span></span>
        <span></span>
        <span></span>
        <span></span>
      </div>
      <div className="file-copy">
        <span className="file-caption">
          {audioPath ? labels.audioFile : labels.noFileSelected}
        </span>
        <span className={`file-path ${audioPath ? "has-file" : ""}`}>
          {audioPath ? getFileName(audioPath) : "MP3 · WAV · M4A"}
        </span>
      </div>
      <button
        className="btn-secondary file-button"
        onClick={pickFile}
        disabled={disabled}
      >
        {labels.chooseFile}
      </button>
    </div>
  );
}
