import { open } from "@tauri-apps/plugin-dialog";

interface FilePickerProps {
  audioPath: string | null;
  onFileSelected: (path: string) => void;
  disabled: boolean;
  labels: {
    chooseFile: string;
    noFileSelected: string;
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
      <button
        className="btn-secondary"
        onClick={pickFile}
        disabled={disabled}
      >
        {labels.chooseFile}
      </button>
      <span className={`file-path ${audioPath ? "has-file" : ""}`}>
        {audioPath ? getFileName(audioPath) : labels.noFileSelected}
      </span>
    </div>
  );
}
