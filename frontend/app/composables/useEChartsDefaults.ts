/**
 * Shared ECharts styling utilities providing glow lines, gradient fills,
 * and frosted-glass tooltips.
 *
 * Uses theme-aware chart colors from `useThemeColors()` and dark-mode
 * detection from `useAppColorMode()`.
 */

/* ---------- private color-space helpers ---------- */

/**
 * Convert a hex color + alpha (0–1) to an rgba() string.
 * ECharts does not reliably support 8-digit hex (#RRGGBBAA),
 * so we must use rgba() for any color with transparency.
 *
 * Defensively validates the input: if the string is not a valid
 * hex color (e.g. if an oklch/color() string leaked through),
 * returns a transparent black fallback to avoid Canvas NaN errors.
 */
function hexToRgba(hex: string, alpha: number): string {
  const stripped = hex.replace('#', '');
  // Validate that we have a 3- or 6-digit hex string
  if (!/^[0-9a-fA-F]{3}$|^[0-9a-fA-F]{6}$/.test(stripped)) {
    return `rgba(0,0,0,${alpha})`;
  }
  let r: number, g: number, b: number;
  if (stripped.length === 3) {
    r = parseInt(stripped[0]! + stripped[0]!, 16);
    g = parseInt(stripped[1]! + stripped[1]!, 16);
    b = parseInt(stripped[2]! + stripped[2]!, 16);
  } else {
    r = parseInt(stripped.substring(0, 2), 16);
    g = parseInt(stripped.substring(2, 4), 16);
    b = parseInt(stripped.substring(4, 6), 16);
  }
  return `rgba(${r},${g},${b},${alpha})`;
}

/* ---------- composable ---------- */

export function useEChartsDefaults() {
  const { isDark } = useAppColorMode();
  const { chart1Color, chart3Color, destructiveColor, successColor } = useThemeColors();

  /** Line style with glow shadow. */
  function glowLineStyle(color: string, width = 2) {
    return { width, color, shadowBlur: 8, shadowColor: hexToRgba(color, 0.5) };
  }

  /** 3-stop vertical gradient fill for area charts. */
  function gradientArea(color: string) {
    return {
      color: {
        type: 'linear',
        x: 0,
        y: 0,
        x2: 0,
        y2: 1,
        colorStops: [
          { offset: 0, color: hexToRgba(color, 0.4) },
          { offset: 0.6, color: hexToRgba(color, 0.15) },
          { offset: 1, color: hexToRgba(color, 0.02) },
        ],
      },
    };
  }

  /** Frosted glass tooltip configuration. */
  function tooltipConfig() {
    return {
      backgroundColor: isDark.value ? 'rgba(24,24,27,0.85)' : 'rgba(255,255,255,0.92)',
      borderColor: isDark.value ? 'rgba(63,63,70,0.6)' : 'rgba(228,228,231,0.8)',
      textStyle: {
        color: isDark.value ? '#fafafa' : '#18181b',
        fontSize: 12,
      },
      extraCssText:
        'backdrop-filter: blur(8px); border-radius: 8px; box-shadow: 0 4px 12px rgba(0,0,0,0.15);',
    };
  }

  /** Emphasis focus on series hover. */
  function emphasisConfig() {
    return { focus: 'series' as const, blurScope: 'coordinateSystem' as const };
  }

  /** Convert hex color + alpha (0–1) to rgba() string for ECharts. */
  function colorAlpha(hex: string, alpha: number): string {
    return hexToRgba(hex, alpha);
  }

  return {
    chart1Color,
    chart3Color,
    destructiveColor,
    successColor,
    glowLineStyle,
    gradientArea,
    tooltipConfig,
    emphasisConfig,
    colorAlpha,
  };
}
