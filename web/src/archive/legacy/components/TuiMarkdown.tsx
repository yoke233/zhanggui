import type { JSX } from "react";
import { TuiCodeBlock } from "@/archive/legacy/components/TuiCodeBlock";

interface TuiMarkdownProps {
  content: string;
  keyPrefix?: string;
}

const parseInlineMarkdown = (text: string, keyPrefix: string): Array<string | JSX.Element> => {
  const nodes: Array<string | JSX.Element> = [];
  const pattern =
    /`([^`]+)`|\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)|\*\*([^*]+)\*\*|(\*[^*]+\*)/g;
  let lastIndex = 0;
  let matchIndex = 0;
  let match = pattern.exec(text);
  while (match) {
    if (match.index > lastIndex) {
      nodes.push(text.slice(lastIndex, match.index));
    }
    if (match[1]) {
      nodes.push(
        <code key={`${keyPrefix}-ic-${matchIndex}`} className="rounded bg-slate-100 px-1 py-0.5 font-mono text-[0.9em] text-slate-900">
          {match[1]}
        </code>,
      );
    } else if (match[2] && match[3]) {
      nodes.push(
        <a key={`${keyPrefix}-a-${matchIndex}`} href={match[3]} target="_blank" rel="noreferrer" className="text-emerald-700 underline decoration-emerald-400/50">
          {match[2]}
        </a>,
      );
    } else if (match[4]) {
      nodes.push(<strong key={`${keyPrefix}-b-${matchIndex}`} className="font-semibold">{match[4]}</strong>);
    } else if (match[5]) {
      nodes.push(<em key={`${keyPrefix}-em-${matchIndex}`} className="italic">{match[5].slice(1, -1)}</em>);
    }
    lastIndex = match.index + match[0].length;
    matchIndex += 1;
    match = pattern.exec(text);
  }
  if (lastIndex < text.length) nodes.push(text.slice(lastIndex));
  if (nodes.length === 0) nodes.push(text);
  return nodes;
};

export function TuiMarkdown({ content, keyPrefix = "md" }: TuiMarkdownProps) {
  const lines = content.replace(/\r\n/g, "\n").split("\n");
  const elements: JSX.Element[] = [];
  let i = 0;

  while (i < lines.length) {
    const line = (lines[i] ?? "").trim();

    if (!line) { i++; continue; }

    // Code block
    if (line.startsWith("```")) {
      const lang = line.slice(3).trim() || undefined;
      const codeLines: string[] = [];
      i++;
      while (i < lines.length && !(lines[i] ?? "").trim().startsWith("```")) {
        codeLines.push(lines[i] ?? "");
        i++;
      }
      i++; // skip closing ```
      elements.push(<TuiCodeBlock key={`${keyPrefix}-cb-${i}`} code={codeLines.join("\n")} language={lang} />);
      continue;
    }

    // Heading
    const headingMatch = line.match(/^(#{1,6})\s+(.+)$/);
    if (headingMatch) {
      const HeadingTag = `h${headingMatch[1].length}` as keyof JSX.IntrinsicElements;
      elements.push(
        <HeadingTag key={`${keyPrefix}-h-${i}`} className="font-semibold leading-snug">
          {parseInlineMarkdown(headingMatch[2], `${keyPrefix}-h-${i}`)}
        </HeadingTag>,
      );
      i++;
      continue;
    }

    // Unordered list
    if (/^[-*]\s+/.test(line)) {
      const items: string[] = [];
      while (i < lines.length) {
        const itemMatch = (lines[i] ?? "").trim().match(/^[-*]\s+(.+)$/);
        if (!itemMatch) break;
        items.push(itemMatch[1]);
        i++;
      }
      elements.push(
        <ul key={`${keyPrefix}-ul-${i}`} className="list-disc space-y-1 pl-5">
          {items.map((item, idx) => (
            <li key={`${keyPrefix}-li-${i}-${idx}`}>{parseInlineMarkdown(item, `${keyPrefix}-li-${i}-${idx}`)}</li>
          ))}
        </ul>,
      );
      continue;
    }

    // Paragraph (collect consecutive non-special lines)
    const paraLines = [line];
    i++;
    while (i < lines.length) {
      const next = (lines[i] ?? "").trim();
      if (!next || /^#{1,6}\s+/.test(next) || /^[-*]\s+/.test(next) || next.startsWith("```")) break;
      paraLines.push(next);
      i++;
    }
    elements.push(
      <p key={`${keyPrefix}-p-${i}`} className="whitespace-pre-wrap">
        {parseInlineMarkdown(paraLines.join("\n"), `${keyPrefix}-p-${i}`)}
      </p>,
    );
  }

  if (elements.length === 0) {
    elements.push(<p key={`${keyPrefix}-empty`} className="whitespace-pre-wrap">{content}</p>);
  }

  return <>{elements}</>;
}
