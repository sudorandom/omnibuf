/** @type {import('tailwindcss').Config} */
export default {
  content: ['./src/**/*.{astro,html,js,jsx,md,mdx,svelte,ts,tsx,vue}'],
  theme: {
    extend: {
      fontFamily: {
        display: ['Space Grotesk', 'sans-serif'],
        serif: ['Instrument Serif', 'serif'],
        mono: ['IBM Plex Mono', 'monospace'],
      },
      colors: {
        paper: '#F4F0E6',
        cream: '#FAF8F2',
        ink: '#111111',
        acid: '#D4FF33',
        orange: '#FF4F18',
        cobalt: '#1E40AF',
        mint: '#10B981',
      },
    },
  },
  plugins: [],
};
