/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['**/*.{html,templ}'],
  theme: {
    colors: {
      red: '#ff4f4f',
      black: '#010101',
      green: '#6bd23b',
      orange: '#fc8d4e',
      yellow: '#f5ba22',
      white: '#fbfbfb',
    },
    extend: {
      colors: {
        redbg: '#ff3838',
        greenbg: '#89e55f',
        yellowbg: '#d09c17',
        white: '#fbfbfb',
        graybg: '#2a2a2a',
        desktopbg: '#1b1b1e',
        toastbg: '#1c3c0e',
        errorbg: '#330f0f',
      },
    },
  },
  plugins: [require('@tailwindcss/forms'), require('@tailwindcss/typography')],
}
