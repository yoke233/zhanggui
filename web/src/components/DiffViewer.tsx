import { useMemo } from "react";
import * as Diff2Html from "diff2html";
import "diff2html/bundles/css/diff2html.min.css";

type DiffOutputFormat = "line-by-line" | "side-by-side";

interface DiffViewerProps {
  diff: string;
  filePath: string;
  outputFormat?: DiffOutputFormat;
}

const DiffViewer = ({
  diff,
  filePath,
  outputFormat = "line-by-line",
}: DiffViewerProps) => {
  const renderedHtml = useMemo(() => {
    const normalized = diff.trim();
    if (!normalized) {
      return "";
    }
    try {
      return Diff2Html.html(normalized, {
        drawFileList: false,
        matching: "lines",
        outputFormat,
      });
    } catch {
      return "";
    }
  }, [diff, outputFormat]);

  return (
    <div className="rounded-md border border-slate-200 bg-white">
      <div className="border-b border-slate-200 px-3 py-2 text-xs font-semibold text-slate-700">
        {filePath}
      </div>
      {renderedHtml ? (
        <div
          className="overflow-x-auto bg-white"
          dangerouslySetInnerHTML={{ __html: renderedHtml }}
        />
      ) : (
        <p className="px-3 py-3 text-xs text-slate-500">暂无 diff 内容。</p>
      )}
    </div>
  );
};

export type { DiffOutputFormat };
export default DiffViewer;
