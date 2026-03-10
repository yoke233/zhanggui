import { useCallback, useState } from "react";

interface QuickInputProps {
  placeholder?: string;
  loading?: boolean;
  onSubmit: (prompt: string) => void | Promise<void>;
}

export function QuickInput({
  placeholder = "描述你的需求，AI 将自动拆解为任务...",
  loading = false,
  onSubmit,
}: QuickInputProps) {
  const [value, setValue] = useState("");

  const submit = useCallback(() => {
    const prompt = value.trim();
    if (!prompt || loading) {
      return;
    }
    void onSubmit(prompt);
    setValue("");
  }, [loading, onSubmit, value]);

  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
      <input
        type="text"
        value={value}
        disabled={loading}
        placeholder={placeholder}
        className="min-w-0 flex-1 rounded-md border border-[#d0d7de] bg-white px-3 py-2 text-sm text-[#24292f] placeholder:text-[#8c959f] focus:border-[#0969da] focus:outline-none focus:ring-2 focus:ring-[#0969da]/15 disabled:cursor-not-allowed disabled:bg-[#f6f8fa]"
        onChange={(event) => {
          setValue(event.target.value);
        }}
        onKeyDown={(event) => {
          if (event.key === "Enter" && !event.shiftKey) {
            event.preventDefault();
            submit();
          }
        }}
      />
      <button
        type="button"
        disabled={loading || value.trim().length === 0}
        className="rounded-md bg-[#0969da] px-4 py-2 text-sm font-medium text-white transition hover:bg-[#0858ba] disabled:cursor-not-allowed disabled:opacity-60"
        onClick={submit}
      >
        {loading ? "拆解中..." : "DAG 拆解"}
      </button>
    </div>
  );
}

export default QuickInput;
