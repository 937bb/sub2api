/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // 主色调 - Apple 系统蓝
        primary: {
          50: '#eef5ff',
          100: '#d9e8ff',
          200: '#b3d1ff',
          300: '#80b3ff',
          400: '#4d94ff',
          500: '#0a84ff',
          600: '#007aff',
          700: '#0062cc',
          800: '#004a99',
          900: '#003166',
          950: '#001f42'
        },
        // 辅助色 - 中性 zinc(原为 slate 蓝,统一去蓝)
        accent: {
          50: '#fafafa',
          100: '#f4f4f5',
          200: '#e4e4e7',
          300: '#d4d4d8',
          400: '#a1a1aa',
          500: '#71717a',
          600: '#52525b',
          700: '#3f3f46',
          800: '#27272a',
          900: '#18181b',
          950: '#09090b'
        },
        // 深色模式背景 - 中性 zinc 灰(去除原 slate 蓝调,避免深色下到处泛蓝)
        dark: {
          50: '#fafafa',
          100: '#f4f4f5',
          200: '#e4e4e7',
          300: '#d4d4d8',
          400: '#a1a1aa',
          500: '#71717a',
          600: '#52525b',
          700: '#3f3f46',
          800: '#27272a',
          900: '#18181b',
          950: '#09090b'
        }
      },
      fontFamily: {
        sans: [
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif'
        ],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace'],
        display: [
          'Bricolage Grotesque',
          'system-ui',
          '-apple-system',
          'PingFang SC',
          'Microsoft YaHei',
          'sans-serif'
        ]
      },
      boxShadow: {
        glass: '0 8px 32px rgba(0, 0, 0, 0.10), inset 0 1px 0 rgba(255, 255, 255, 0.5)',
        'glass-sm': '0 4px 16px rgba(0, 0, 0, 0.08), inset 0 1px 0 rgba(255, 255, 255, 0.4)',
        'glass-lg': '0 16px 48px rgba(0, 0, 0, 0.14), inset 0 1px 0 rgba(255, 255, 255, 0.55)',
        glow: '0 0 20px rgba(10, 132, 255, 0.25)',
        'glow-lg': '0 0 40px rgba(10, 132, 255, 0.35)',
        card: '0 1px 3px rgba(0, 0, 0, 0.04), 0 1px 2px rgba(0, 0, 0, 0.06)',
        'card-hover': '0 10px 40px rgba(0, 0, 0, 0.08)',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.1)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #0a84ff 0%, #007aff 100%)',
        'gradient-dark': 'linear-gradient(135deg, #27272a 0%, #18181b 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.25) 0%, rgba(255,255,255,0.06) 100%)',
        'mesh-gradient':
          'radial-gradient(at 40% 20%, rgba(10, 132, 255, 0.14) 0px, transparent 50%), radial-gradient(at 80% 0%, rgba(90, 200, 250, 0.10) 0px, transparent 50%), radial-gradient(at 0% 50%, rgba(0, 122, 255, 0.10) 0px, transparent 50%)'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'glow 2s ease-in-out infinite alternate'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        },
        glow: {
          '0%': { boxShadow: '0 0 20px rgba(10, 132, 255, 0.25)' },
          '100%': { boxShadow: '0 0 30px rgba(10, 132, 255, 0.4)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      },
      borderRadius: {
        '4xl': '2rem'
      }
    }
  },
  plugins: []
}
