import { Box, Paper, Typography } from "@mui/material";
import { CopyButton } from "./CopyButton";

export function CodeSurface({
  filename,
  tag,
  code,
  footer,
  status,
  copyable = true,
  dense = false,
  fill = false,
  sx
}) {
  return (
    <Paper
      variant="outlined"
      sx={[
        {
          borderColor: "divider",
          borderRadius: 1.25,
          overflow: "hidden",
          bgcolor: "background.paper",
          boxShadow: 1,
          ...(fill ? { height: "100%", display: "flex", flexDirection: "column" } : {})
        },
        ...(Array.isArray(sx) ? sx : [sx])
      ]}
    >
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 2,
          minHeight: 42,
          px: 2,
          py: 0.5,
          bgcolor: "action.hover",
          borderBottom: 1,
          borderColor: "divider"
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, minWidth: 0 }}>
          <Box sx={{ width: 7, height: 7, borderRadius: "50%", bgcolor: "primary.main", flex: "none" }} />
          <Typography
            component="span"
            noWrap
            sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.71875rem", color: "text.secondary" }}
          >
            {filename || "text"}
          </Typography>
        </Box>
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, flex: "none" }}>
          {tag ? (
            <Typography
              component="span"
              sx={{ display: { xs: "none", sm: "inline" }, fontFamily: (theme) => theme.fonts.mono, fontSize: "0.65625rem", color: "text.secondary" }}
            >
              {tag}
            </Typography>
          ) : null}
          {copyable ? <CopyButton text={code} /> : null}
        </Box>
      </Box>
      <Box
        component="pre"
        tabIndex={0}
        aria-label={`${filename || "Code"} block`}
        sx={{
          m: 0,
          p: dense ? 1.75 : 2.25,
          overflowX: "auto",
          fontFamily: (theme) => theme.fonts.mono,
          fontSize: dense ? "0.71875rem" : "0.78125rem",
          lineHeight: 1.8,
          color: "text.primary",
          bgcolor: "background.paper",
          ...(fill ? { flex: 1 } : {})
        }}
      >
        <Box component="code" sx={{ font: "inherit" }}>{code}</Box>
      </Box>
      {footer || status ? (
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 1.5,
            minHeight: 40,
            px: 2,
            py: 1,
            borderTop: 1,
            borderColor: "divider",
            bgcolor: "action.hover"
          }}
        >
          <Typography component="span" sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.65625rem", color: "text.secondary" }}>
            {footer}
          </Typography>
          {status ? (
            <Box sx={{ display: "inline-flex", alignItems: "center", gap: 0.75, flex: "none" }}>
              <Box sx={{ width: 5, height: 5, borderRadius: "50%", bgcolor: "success.main" }} />
              <Typography component="span" sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.65625rem", color: "success.main" }}>
                {status}
              </Typography>
            </Box>
          ) : null}
        </Box>
      ) : null}
    </Paper>
  );
}
