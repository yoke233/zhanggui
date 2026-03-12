import { useEffect, useRef, useState } from "react";

interface Props {
  containerRef: React.RefObject<HTMLDivElement | null>;
}

/**
 * Overlays the native scrollbar with small blue markers indicating
 * where user messages are in the scroll timeline.
 * The component is purely visual (pointer-events: none) so it
 * does not interfere with the native scrollbar interaction.
 */
export function ChatScrollTrack({ containerRef }: Props) {
  const [markers, setMarkers] = useState<number[]>([]);
  const rafRef = useRef<number | null>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const compute = () => {
      if (rafRef.current != null) return; // debounce to next frame
      rafRef.current = requestAnimationFrame(() => {
        rafRef.current = null;
        const { scrollHeight } = container;
        if (scrollHeight === 0) return;
        const msgs = container.querySelectorAll<HTMLElement>("[data-user-msg]");
        setMarkers(
          Array.from(msgs).map((el) => (el.offsetTop / scrollHeight) * 100),
        );
      });
    };

    compute();

    const ro = new ResizeObserver(compute);
    ro.observe(container);

    // Watch for new messages being added
    const mo = new MutationObserver(compute);
    mo.observe(container, { childList: true, subtree: true });

    return () => {
      if (rafRef.current != null) cancelAnimationFrame(rafRef.current);
      ro.disconnect();
      mo.disconnect();
    };
  }, [containerRef]);

  if (markers.length === 0) return null;

  return (
    <div
      aria-hidden
      className="pointer-events-none absolute bottom-0 right-0 top-0 w-3.5 z-10"
    >
      {markers.map((pct, i) => (
        <div
          key={i}
          className="absolute right-0.5 h-[4px] w-2 rounded-sm bg-blue-400/80"
          style={{ top: `calc(${pct}% - 2px)` }}
        />
      ))}
    </div>
  );
}
