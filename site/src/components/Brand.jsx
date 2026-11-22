import { Box, Link, SvgIcon, Typography } from "@mui/material";

export function LeverMark(props) {
  return (
    <SvgIcon viewBox="0 0 24 24" {...props}>
      <path d="M5 19h14M12 19V5" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="square" />
      <path d="M9 19l3-6 3 6" fill="currentColor" />
    </SvgIcon>
  );
}

export function Brand({ href = "/", sublabel, onClick, compact = false }) {
  return (
    <Link
      href={href}
      onClick={onClick}
      underline="none"
      sx={{
        display: "inline-flex",
        alignItems: "center",
        gap: 1.25,
        flex: "none",
        minWidth: 0,
        color: "text.primary",
        "&:hover": { color: "text.primary" }
      }}
    >
      <Box
        aria-hidden="true"
        sx={{
          display: "grid",
          placeItems: "center",
          width: 28,
          height: 28,
          borderRadius: "7px",
          bgcolor: "text.primary",
          color: "background.paper",
          flex: "none"
        }}
      >
        <LeverMark sx={{ fontSize: 17 }} />
      </Box>
      <Box sx={{ display: "flex", alignItems: "baseline", gap: 1.25, minWidth: 0 }}>
        <Typography
          component="span"
          sx={{
            fontFamily: (theme) => theme.fonts.body,
            fontSize: "0.875rem",
            fontWeight: 600,
            letterSpacing: "0.15em",
            lineHeight: 1,
            whiteSpace: "nowrap"
          }}
        >
          LEVIER
          <Box component="span" sx={{ color: "primary.main" }}>.</Box>
        </Typography>
        {!compact && sublabel ? (
          <Typography
            component="span"
            sx={{
              display: { xs: "none", sm: "inline" },
              pl: 1.5,
              ml: 0.25,
              borderLeft: 1,
              borderColor: "divider",
              fontSize: "0.75rem",
              color: "text.secondary",
              lineHeight: 1.4,
              whiteSpace: "nowrap"
            }}
          >
            {sublabel}
          </Typography>
        ) : null}
      </Box>
    </Link>
  );
}
