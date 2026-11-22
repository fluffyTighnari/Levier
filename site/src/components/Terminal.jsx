import { Box, Typography } from "@mui/material";
import { alpha } from "@mui/material/styles";
import { motion, useReducedMotion } from "motion/react";

const LINE_COLORS = {
  cmd: "terminal.fg",
  out: "terminal.fg",
  ok: "terminal.dim",
  dim: "terminal.muted"
};

export function Terminal({ step, animate = false }) {
  const reduced = useReducedMotion();
  const shouldAnimate = animate && !reduced;

  return (
    <Box
      sx={{
        border: 1,
        borderColor: "terminal.bg",
        borderRadius: 1.25,
        bgcolor: "terminal.bg",
        color: "terminal.fg",
        overflow: "hidden",
        boxShadow: 1
      }}
    >
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 2,
          px: 1.75,
          py: 1.25,
          borderBottom: 1,
          borderBottomColor: (theme) => alpha(theme.palette.terminal.fg, 0.16)
        }}
      >
        <Typography sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.65625rem", color: "terminal.muted" }}>
          levier — session
        </Typography>
        <Box sx={{ display: "inline-flex", alignItems: "center", gap: 0.75 }}>
          <Box sx={{ width: 5, height: 5, borderRadius: "50%", bgcolor: "primary.main" }} />
          <Typography sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.625rem", color: "terminal.dim" }}>
            LIVE
          </Typography>
        </Box>
      </Box>

      <Box
        component="pre"
        tabIndex={0}
        aria-label="CLI session output"
        sx={{
          m: 0,
          p: 2.5,
          minHeight: { xs: 300, md: 356 },
          overflowX: "auto",
          fontFamily: (theme) => theme.fonts.mono,
          fontSize: { xs: "0.71875rem", md: "0.75rem" },
          lineHeight: 1.85,
          whiteSpace: "pre-wrap",
          wordBreak: "break-word"
        }}
      >
        {step.lines.map((line, index) => (
          <Box
            key={`${step.id}-${index}`}
            component={motion.span}
            initial={shouldAnimate ? { opacity: 0, y: 4 } : false}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.22, delay: shouldAnimate ? index * 0.055 : 0 }}
            sx={{
              display: "block",
              color: line.type === "out" ? "terminal.fg" : LINE_COLORS[line.type] || "terminal.fg",
              opacity: line.type === "out" ? 0.76 : 1
            }}
          >
            {line.segments.map((segment, segmentIndex) => (
              <Box
                component="span"
                key={`${index}-${segmentIndex}`}
                sx={{
                  color:
                    segment.tone === "prompt"
                      ? "terminal.accent"
                      : segment.tone === "flag"
                        ? "terminal.flag"
                        : "inherit"
                }}
              >
                {segment.text}
              </Box>
            ))}
          </Box>
        ))}
      </Box>
    </Box>
  );
}
