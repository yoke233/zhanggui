import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  theme: {
    extend: {
      keyframes: {
        "dialog-overlay-in": {
          from: { opacity: "0" },
          to: { opacity: "1" },
        },
        "dialog-content-in": {
          from: { opacity: "0", transform: "scale(0.96) translateY(6px)" },
          to: { opacity: "1", transform: "scale(1) translateY(0)" },
        },
        "select-in": {
          from: { opacity: "0", transform: "scale(0.97) translateY(-2px)" },
          to: { opacity: "1", transform: "scale(1) translateY(0)" },
        },
      },
      animation: {
        "dialog-overlay-in": "dialog-overlay-in 0.18s ease-out",
        "dialog-content-in": "dialog-content-in 0.18s ease-out",
        "select-in": "select-in 0.12s ease-out",
      },
    },
  },
  plugins: []
} satisfies Config;
