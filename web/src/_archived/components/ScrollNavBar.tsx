import { useState, useCallback } from "react";

export interface ScrollMarker {
  id: string;
  label: string;
  /** 0-1，表示在总滚动高度中的相对位置 */
  position: number;
}

interface ScrollNavBarProps {
  markers: ScrollMarker[];
  onMarkerClick: (id: string) => void;
}

export function ScrollNavBar({ markers, onMarkerClick }: ScrollNavBarProps) {
  const [hoveredId, setHoveredId] = useState<string | null>(null);

  const handleClick = useCallback(
    (id: string) => {
      onMarkerClick(id);
    },
    [onMarkerClick],
  );

  return (
    <div className="relative h-full w-3 shrink-0 bg-slate-100">
      {markers.map((marker) => (
        <div
          key={marker.id}
          className="absolute left-1/2 -translate-x-1/2"
          style={{ top: `${marker.position * 100}%` }}
        >
          <button
            type="button"
            className="h-2 w-2 rounded-full bg-blue-400 transition-transform hover:scale-150 hover:bg-blue-500"
            onClick={() => handleClick(marker.id)}
            onMouseEnter={() => setHoveredId(marker.id)}
            onMouseLeave={() => setHoveredId(null)}
            aria-label={marker.label}
          />
          {hoveredId === marker.id && (
            <div className="absolute right-4 top-1/2 z-20 -translate-y-1/2 whitespace-nowrap rounded bg-slate-800 px-2 py-1 text-xs text-white shadow-lg">
              {marker.label}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
