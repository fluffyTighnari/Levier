import { alpha, createTheme } from "@mui/material/styles";
import { colors, fonts, radius } from "./tokens";

const focusRing = {
  outline: `2px solid ${colors.accent}`,
  outlineOffset: 2
};

const softShadow = `0 1px 2px ${alpha(colors.ink, 0.06)}`;
const shadows = Array.from({ length: 25 }, (_, index) => (index === 0 ? "none" : softShadow));

export const theme = createTheme({
  cssVariables: true,
  palette: {
    mode: "light",
    primary: {
      main: colors.accent,
      dark: colors.accentDark,
      light: colors.accentSoft,
      contrastText: colors.onAccent
    },
    secondary: {
      main: colors.info,
      contrastText: colors.onAccent
    },
    success: { main: colors.success },
    warning: { main: colors.warning },
    error: { main: colors.error },
    info: { main: colors.info },
    text: {
      primary: colors.ink,
      secondary: colors.ink2,
      disabled: colors.disabled
    },
    background: {
      default: colors.canvas,
      paper: colors.paper
    },
    divider: colors.line,
    action: {
      hover: colors.sunken,
      selected: colors.accentSoft,
      disabled: alpha(colors.ink, 0.38),
      disabledBackground: alpha(colors.ink, 0.12),
      focus: alpha(colors.accent, 0.16)
    },
    terminal: {
      bg: colors.terminal,
      fg: colors.terminalInk,
      muted: colors.terminalMuted,
      dim: colors.terminalDim,
      accent: colors.terminalAccent,
      flag: colors.terminalFlag
    }
  },
  shape: { borderRadius: 8 },
  spacing: 8,
  shadows,
  typography: {
    fontFamily: fonts.body,
    fontWeightLight: 300,
    fontWeightRegular: 400,
    fontWeightMedium: 500,
    fontWeightBold: 600,
    h1: {
      fontFamily: fonts.display,
      fontWeight: 500,
      fontSize: "clamp(2.5rem, 4.4vw, 3.7rem)",
      lineHeight: 1.06,
      letterSpacing: "-0.028em"
    },
    h2: {
      fontFamily: fonts.display,
      fontWeight: 500,
      fontSize: "clamp(1.65rem, 2.3vw, 1.9rem)",
      lineHeight: 1.18,
      letterSpacing: "-0.014em"
    },
    h3: {
      fontFamily: fonts.display,
      fontWeight: 500,
      fontSize: "clamp(2rem, 3.4vw, 2.4rem)",
      lineHeight: 1.12,
      letterSpacing: "-0.022em"
    },
    h4: {
      fontWeight: 600,
      fontSize: "1.0625rem",
      lineHeight: 1.4,
      letterSpacing: 0
    },
    h5: {
      fontWeight: 600,
      fontSize: "0.9375rem",
      lineHeight: 1.4
    },
    h6: {
      fontWeight: 600,
      fontSize: "0.875rem",
      lineHeight: 1.4
    },
    body1: {
      fontSize: "1rem",
      lineHeight: 1.75,
      letterSpacing: 0
    },
    body2: {
      fontSize: "0.875rem",
      lineHeight: 1.7,
      letterSpacing: 0
    },
    button: {
      fontSize: "0.875rem",
      fontWeight: 600,
      lineHeight: 1.2,
      letterSpacing: 0,
      textTransform: "none"
    },
    caption: {
      fontSize: "0.75rem",
      lineHeight: 1.5,
      letterSpacing: 0
    },
    overline: {
      fontSize: "0.6875rem",
      fontWeight: 500,
      lineHeight: 1.5,
      letterSpacing: "0.06em",
      textTransform: "uppercase"
    }
  },
  components: {
    MuiCssBaseline: {
      styleOverrides: {
        html: {
          scrollBehavior: "smooth",
          scrollPaddingTop: 84
        },
        body: {
          backgroundColor: colors.canvas,
          color: colors.ink2,
          fontFamily: fonts.body,
          WebkitFontSmoothing: "antialiased",
          textRendering: "optimizeLegibility"
        },
        "::selection": {
          backgroundColor: colors.accent,
          color: colors.onAccent
        },
        "code, pre, kbd, samp": {
          fontFamily: fonts.mono
        },
        "*": {
          scrollbarWidth: "thin",
          scrollbarColor: `${colors.lineStrong} transparent`
        },
        "*::-webkit-scrollbar": {
          width: 10,
          height: 10
        },
        "*::-webkit-scrollbar-track": {
          background: "transparent"
        },
        "*::-webkit-scrollbar-thumb": {
          backgroundColor: colors.lineStrong,
          border: `3px solid ${colors.canvas}`,
          borderRadius: 8
        },
        ":focus-visible": focusRing,
        "[data-focus-silent]:focus, [data-focus-silent]:focus-visible": {
          outline: "none"
        },
        "@media (prefers-reduced-motion: reduce)": {
          html: { scrollBehavior: "auto" },
          "*, *::before, *::after": {
            animationDuration: "0.01ms !important",
            animationIterationCount: "1 !important",
            transitionDuration: "0.01ms !important"
          }
        }
      }
    },
    MuiAppBar: {
      defaultProps: { color: "transparent", elevation: 0, position: "sticky" },
      styleOverrides: {
        root: {
          backgroundColor: colors.paper,
          backgroundImage: "none",
          borderBottom: `1px solid ${colors.line}`,
          color: colors.ink
        }
      }
    },
    MuiToolbar: {
      styleOverrides: {
        root: {
          minHeight: 64,
          paddingInline: 24,
          gap: 16
        }
      }
    },
    MuiPaper: {
      defaultProps: { elevation: 0 },
      styleOverrides: {
        root: { backgroundImage: "none" }
      }
    },
    MuiButton: {
      defaultProps: { disableElevation: true },
      styleOverrides: {
        root: {
          borderRadius: radius.control,
          minHeight: 40,
          paddingInline: 16,
          fontWeight: 600,
          transition: "background-color 160ms ease, border-color 160ms ease, color 160ms ease, transform 80ms ease",
          "&:active": { transform: "translateY(1px)" },
          "&.Mui-focusVisible": focusRing
        },
        sizeSmall: { minHeight: 34, fontSize: "0.8125rem", paddingInline: 12 },
        sizeLarge: { minHeight: 46, fontSize: "0.9rem", paddingInline: 22 },
        containedPrimary: {
          backgroundColor: colors.accent,
          color: colors.onAccent,
          "&:hover": { backgroundColor: colors.accentDark }
        },
        outlined: {
          borderColor: colors.lineStrong,
          color: colors.ink,
          "&:hover": { backgroundColor: colors.sunken, borderColor: colors.ink2 }
        },
        text: {
          color: colors.ink,
          "&:hover": { backgroundColor: colors.sunken }
        }
      }
    },
    MuiIconButton: {
      styleOverrides: {
        root: {
          borderRadius: radius.control,
          color: colors.ink2,
          transition: "background-color 160ms ease, color 160ms ease",
          "&:hover": { backgroundColor: colors.sunken, color: colors.ink },
          "&:active": { backgroundColor: colors.sunkenStrong },
          "&.Mui-focusVisible": focusRing,
          "&.Mui-disabled": { color: colors.disabled }
        }
      }
    },
    MuiLink: {
      defaultProps: { underline: "hover" },
      styleOverrides: {
        root: {
          color: "inherit",
          textUnderlineOffset: "0.18em",
          textDecorationThickness: 1,
          "&:hover": { color: colors.accent }
        }
      }
    },
    MuiDrawer: {
      styleOverrides: {
        paper: {
          backgroundColor: colors.canvas,
          borderRight: `1px solid ${colors.ink}`,
          backgroundImage: "none",
          boxShadow: "none"
        }
      }
    },
    MuiTabs: {
      styleOverrides: {
        root: { minHeight: 44 },
        indicator: { height: 2 }
      }
    },
    MuiTab: {
      styleOverrides: {
        root: {
          minHeight: 44,
          paddingInline: 12,
          fontSize: "0.875rem",
          fontWeight: 500,
          textTransform: "none",
          "&.Mui-selected": { fontWeight: 600 },
          "&.Mui-focusVisible": { outline: `2px solid ${colors.accent}`, outlineOffset: -2 }
        }
      }
    },
    MuiTableCell: {
      styleOverrides: {
        root: {
          borderColor: colors.line,
          padding: "10px 12px",
          fontSize: "0.8125rem",
          lineHeight: 1.6
        },
        head: {
          backgroundColor: colors.sunken,
          color: colors.ink,
          fontSize: "0.75rem",
          fontWeight: 600,
          whiteSpace: "nowrap",
          borderBottomColor: colors.lineStrong
        },
        body: { color: colors.ink2 }
      }
    },
    MuiAlert: {
      styleOverrides: {
        root: {
          borderRadius: radius.control,
          border: `1px solid ${colors.line}`,
          padding: "12px 14px",
          alignItems: "flex-start",
          fontSize: "0.875rem",
          lineHeight: 1.7
        },
        icon: { marginTop: 2, marginRight: 10, padding: 0 },
        message: { padding: 0, width: "100%" },
        action: { padding: 0, marginLeft: 8 }
      }
    },
    MuiChip: {
      styleOverrides: {
        root: {
          height: 26,
          borderRadius: radius.control,
          fontSize: "0.75rem",
          borderColor: colors.lineStrong,
          color: colors.ink2,
          backgroundColor: "transparent"
        },
        label: { paddingInline: 8 },
        icon: { marginLeft: 4, marginRight: -2 }
      }
    },
    MuiTextField: {
      defaultProps: { size: "small" }
    },
    MuiOutlinedInput: {
      styleOverrides: {
        root: {
          borderRadius: radius.control,
          backgroundColor: colors.paper,
          fontSize: "0.8125rem",
          "& .MuiOutlinedInput-notchedOutline": { borderColor: colors.line },
          "&:hover .MuiOutlinedInput-notchedOutline": { borderColor: colors.lineStrong },
          "&.Mui-focused .MuiOutlinedInput-notchedOutline": {
            borderColor: colors.accent,
            borderWidth: 1,
            boxShadow: `0 0 0 1px ${colors.accent}`
          }
        }
      }
    },
    MuiAccordion: {
      defaultProps: { elevation: 0, disableGutters: true },
      styleOverrides: {
        root: {
          border: `1px solid ${colors.line}`,
          borderRadius: radius.control,
          backgroundColor: colors.paper,
          "&:before": { display: "none" },
          "&.Mui-expanded": { margin: 0 }
        }
      }
    },
    MuiAccordionSummary: {
      styleOverrides: {
        root: { minHeight: 44, paddingInline: 12 },
        content: { margin: "10px 0" }
      }
    },
    MuiAccordionDetails: {
      styleOverrides: { root: { padding: "0 12px 12px" } }
    },
    MuiSnackbarContent: {
      styleOverrides: {
        root: {
          backgroundColor: colors.ink,
          color: colors.paper,
          boxShadow: "none",
          border: `1px solid ${colors.lineStrong}`,
          borderRadius: radius.control,
          fontSize: "0.8125rem"
        }
      }
    },
    MuiTooltip: {
      styleOverrides: {
        tooltip: {
          backgroundColor: colors.ink,
          color: colors.paper,
          fontSize: "0.75rem",
          borderRadius: radius.control,
          padding: "6px 9px"
        },
        arrow: { color: colors.ink }
      }
    },
    MuiSkeleton: {
      styleOverrides: {
        root: { backgroundColor: colors.sunken, borderRadius: 4 }
      }
    },
    MuiListItemButton: {
      styleOverrides: {
        root: {
          borderRadius: radius.control,
          minHeight: 34,
          padding: "5px 10px",
          fontSize: "0.8125rem",
          lineHeight: 1.45,
          color: colors.ink2,
          "&:hover": { backgroundColor: colors.sunken, color: colors.ink },
          "&.Mui-selected": {
            backgroundColor: colors.accentSoft,
            color: colors.ink,
            fontWeight: 600,
            boxShadow: `inset 2px 0 0 ${colors.accent}`,
            "&:hover": { backgroundColor: colors.accentSoft }
          },
          "&.Mui-focusVisible": { outline: `2px solid ${colors.accent}`, outlineOffset: 1 }
        }
      }
    },
    MuiBreadcrumbs: {
      styleOverrides: {
        separator: { color: colors.lineStrong, marginInline: 6 },
        li: { fontSize: "0.75rem" }
      }
    },
    MuiDivider: {
      styleOverrides: { root: { borderColor: colors.line } }
    },
    MuiMenu: {
      styleOverrides: {
        paper: {
          border: `1px solid ${colors.line}`,
          boxShadow: softShadow,
          backgroundColor: colors.paper
        }
      }
    },
    MuiMenuItem: {
      styleOverrides: { root: { fontSize: "0.8125rem" } }
    }
  }
});

/* Font stacks are exposed on the theme so every component can reference them
   through sx instead of repeating CSS font stacks. */
theme.fonts = fonts;
