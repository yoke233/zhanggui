import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { FileText, Image as ImageIcon, Loader2, Paperclip, Upload, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface UploadedAttachment {
  id: number;
  file_name: string;
  mime_type: string;
}

interface AttachmentUploaderProps {
  pendingFiles: File[];
  uploadedAttachments: UploadedAttachment[];
  uploading: boolean;
  allowedExtensions?: string[];
  onFilesSelected: (files: File[]) => void;
  onRemovePending: (index: number) => void;
  onRemoveUploaded: (attachment: UploadedAttachment) => void;
}

export function AttachmentUploader({
  pendingFiles,
  uploadedAttachments,
  uploading,
  allowedExtensions = [".md", ".txt", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg"],
  onFilesSelected,
  onRemovePending,
  onRemoveUploaded,
}: AttachmentUploaderProps) {
  const { t } = useTranslation();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [dragOver, setDragOver] = useState(false);

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    onFilesSelected(Array.from(e.dataTransfer.files));
  };

  const formatSize = (size: number) => {
    if (size < 1024) return `${size}B`;
    if (size < 1024 * 1024) return `${(size / 1024).toFixed(0)}KB`;
    return `${(size / (1024 * 1024)).toFixed(1)}MB`;
  };

  return (
    <div className="space-y-1.5">
      <label className="text-sm font-medium">{t("createWorkItem.attachments")}</label>
      <div
        className={cn(
          "relative flex min-h-[100px] flex-col items-center justify-center rounded-lg border-2 border-dashed px-4 py-6 transition-colors",
          dragOver
            ? "border-primary bg-primary/5"
            : "border-muted-foreground/25 hover:border-muted-foreground/50",
        )}
        onDrop={handleDrop}
        onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
        onDragLeave={(e) => { e.preventDefault(); setDragOver(false); }}
      >
        <input
          ref={fileInputRef}
          type="file"
          className="hidden"
          multiple
          accept={allowedExtensions.join(",")}
          onChange={(e) => {
            if (e.target.files) {
              onFilesSelected(Array.from(e.target.files));
              e.target.value = "";
            }
          }}
        />
        {dragOver ? (
          <div className="flex flex-col items-center gap-2 text-primary">
            <Upload className="h-8 w-8" />
            <span className="text-sm font-medium">{t("createWorkItem.dropFilesHere")}</span>
          </div>
        ) : (
          <div className="flex flex-col items-center gap-2 text-muted-foreground">
            <Paperclip className="h-6 w-6" />
            <span className="text-xs">{t("createWorkItem.attachmentsDesc")}</span>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="mt-1"
              onClick={() => fileInputRef.current?.click()}
            >
              {t("createWorkItem.browseFiles")}
            </Button>
          </div>
        )}
      </div>

      {/* Pending files */}
      {pendingFiles.length > 0 && (
        <div className="flex flex-wrap gap-2 pt-2">
          {pendingFiles.map((file, idx) => {
            const isImage = file.type.startsWith("image/");
            return (
              <div
                key={`pending-${idx}`}
                className="group flex items-center gap-1.5 rounded-md border bg-muted/50 px-2.5 py-1.5 text-xs"
              >
                {isImage ? (
                  <ImageIcon className="h-3.5 w-3.5 text-green-600" />
                ) : (
                  <FileText className="h-3.5 w-3.5 text-blue-600" />
                )}
                <span className="max-w-[120px] truncate">{file.name}</span>
                <span className="text-muted-foreground">{formatSize(file.size)}</span>
                <button
                  type="button"
                  onClick={() => onRemovePending(idx)}
                  className="ml-0.5 text-muted-foreground hover:text-red-500"
                >
                  <X className="h-3 w-3" />
                </button>
              </div>
            );
          })}
        </div>
      )}

      {/* Uploaded attachments */}
      {uploadedAttachments.length > 0 && (
        <div className="flex flex-wrap gap-2 pt-1">
          {uploadedAttachments.map((att) => {
            const isImage = att.mime_type.startsWith("image/");
            return (
              <div
                key={`uploaded-${att.id}`}
                className="group flex items-center gap-1.5 rounded-md border border-green-200 bg-green-50 px-2.5 py-1.5 text-xs"
              >
                {isImage ? (
                  <ImageIcon className="h-3.5 w-3.5 text-green-600" />
                ) : (
                  <FileText className="h-3.5 w-3.5 text-blue-600" />
                )}
                <span className="max-w-[120px] truncate">{att.file_name}</span>
                <span className="inline-flex h-3 w-3 items-center justify-center text-green-600">&#10003;</span>
                <button
                  type="button"
                  onClick={() => onRemoveUploaded(att)}
                  className="ml-0.5 text-muted-foreground hover:text-red-500"
                >
                  <X className="h-3 w-3" />
                </button>
              </div>
            );
          })}
        </div>
      )}

      {uploading && (
        <div className="flex items-center gap-2 pt-1 text-xs text-muted-foreground">
          <Loader2 className="h-3 w-3 animate-spin" />
          {t("createWorkItem.uploadingFiles")}
        </div>
      )}
    </div>
  );
}
