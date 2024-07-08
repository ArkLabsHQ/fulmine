/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['**/*.{html,templ}'],
  theme: {
    colors: {
      red: '#ff4f4f',
      green: '#6bd23b',
      yellow: '#f5ba22',
    },
    extend: {
      colors: {
        redbg: '#ff3838',
        greenbg: '#89e55f',
        yellowbg: '#d09c17',
        offwhite: '#fbfbfb',
        graybg: '#2a2a2a',
      },
    },
  },
  plugins: [require('@tailwindcss/forms'), require('@tailwindcss/typography')],
}
