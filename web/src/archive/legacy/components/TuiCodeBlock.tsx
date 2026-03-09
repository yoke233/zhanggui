import { useState, useCallback } from "react";
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter";
import { oneDark } from "react-syntax-highlighter/dist/esm/styles/prism";

interface TuiCodeBlockProps {
  code: string;
  language?: string;
}

export function TuiCodeBlock({ code, language }: TuiCodeBlockProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    void navigator.clipboard.writeText(code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }, [code]);

  return (
    <div className="group relative my-2 rounded-md bg-slate-900">
      <div className="flex items-center justify-between px-3 py-1 text-xs text-slate-400">
        {language ? <span>{language}</span> : <span />}
        <button
          type="button"
          aria-label="复制代码"
          className="rounded px-2 py-0.5 text-slate-400 hover:bg-slate-700 hover:text-slate-200"
          onClick={handleCopy}
        >
          {copied ? "已复制" : "复制"}
        </button>
      </div>
      <SyntaxHighlighter
        language={language || "text"}
        style={oneDark}
        customStyle={{ margin: 0, padding: "0.5rem 0.75rem", background: "transparent", fontSize: "0.75rem" }}
        wrapLongLines
      >
        {code}
      </SyntaxHighlighter>
    </div>
  );
}
