import { Box, Container, Link, Typography } from "@mui/material";
import { alpha } from "@mui/material/styles";

const DOC_LINKS = [
  ["Full setup", "./docs/index.html#/onboarding"],
  ["Model configuration", "./docs/index.html#/pre-model"],
  ["App core workflow", "./docs/index.html#/app-core"],
  ["Install and start", "./docs/index.html#/levier-install"],
  ["Runtime core workflow", "./docs/index.html#/runtime-core"],
  ["CLI reference", "./docs/index.html#/cli"]
];

const PROJECT_LINKS = [
  ["GitHub", "https://github.com/fluffyTighnari/levier"],
  ["README", "https://github.com/fluffyTighnari/levier/blob/main/README.md"],
  ["QUICKSTART", "https://github.com/fluffyTighnari/levier/blob/main/docs/QUICKSTART.md"],
  ["SCOPE", "https://github.com/fluffyTighnari/levier/blob/main/docs/SCOPE.md"]
];

export function SiteFooter() {
  return (
    <Box component="footer" sx={{ bgcolor: "terminal.bg", color: "terminal.fg" }}>
      <Container maxWidth="lg" sx={{ py: { xs: 6, md: 7.5 } }}>
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr", md: "1.5fr 0.75fr 0.75fr 1fr" },
            gap: { xs: 4, md: 6 },
            pb: 5,
            borderBottom: 1,
            borderColor: (theme) => alpha(theme.palette.terminal.fg, 0.16)
          }}
        >
          <Box>
            <Typography sx={{ fontSize: "1.4375rem", fontWeight: 600, letterSpacing: "0.14em", lineHeight: 1 }}>
              LEVIER<Box component="span" sx={{ color: "terminal.accent" }}>.</Box>
            </Typography>
            <Typography sx={{ mt: 2, fontSize: "0.8125rem", lineHeight: 1.75, color: (theme) => alpha(theme.palette.terminal.fg, 0.72) }}>
              Runtime, designed by you.<br />One run is enough.
            </Typography>
            <Typography sx={{ mt: 2, fontFamily: (theme) => theme.fonts.mono, fontSize: "0.65625rem", color: (theme) => alpha(theme.palette.terminal.fg, 0.72) }}>
              Levier — French for lever.
            </Typography>
          </Box>

          <Box component="nav" aria-label="Documentation">
            <Typography variant="overline" sx={{ display: "block", mb: 1.5, color: (theme) => alpha(theme.palette.terminal.fg, 0.66) }}>
              Documentation
            </Typography>
            <Box sx={{ display: "grid", gap: 0.9 }}>
              {DOC_LINKS.map(([label, href]) => (
                <Link key={label} href={href} underline="none" sx={{ fontSize: "0.8125rem", color: (theme) => alpha(theme.palette.terminal.fg, 0.84), "&:hover": { color: "terminal.accent" } }}>
                  {label}
                </Link>
              ))}
            </Box>
          </Box>

          <Box component="nav" aria-label="Project">
            <Typography variant="overline" sx={{ display: "block", mb: 1.5, color: (theme) => alpha(theme.palette.terminal.fg, 0.66) }}>
              Project
            </Typography>
            <Box sx={{ display: "grid", gap: 0.9 }}>
              {PROJECT_LINKS.map(([label, href]) => (
                <Link key={label} href={href} target="_blank" rel="noopener noreferrer" underline="none" sx={{ fontSize: "0.8125rem", color: (theme) => alpha(theme.palette.terminal.fg, 0.84), "&:hover": { color: "terminal.accent" } }}>
                  {label} ↗
                </Link>
              ))}
            </Box>
          </Box>

          <Box>
            <Box component="span" sx={{ display: "inline-flex", px: 0.875, py: 0.375, border: 1, borderRadius: 0.75, borderColor: (theme) => alpha(theme.palette.terminal.fg, 0.24), fontFamily: (theme) => theme.fonts.mono, fontSize: "0.625rem", color: (theme) => alpha(theme.palette.terminal.fg, 0.72) }}>
              2026 · MVP
            </Box>
            <Typography sx={{ mt: 1.5, fontSize: "0.75rem", lineHeight: 1.75, color: (theme) => alpha(theme.palette.terminal.fg, 0.66) }}>
              A single-machine stack. The default Runtime and MCP components ship with the product and can be replaced against the contract.
            </Typography>
            <Typography sx={{ mt: 1.25, fontSize: "0.75rem", lineHeight: 1.75, color: (theme) => alpha(theme.palette.terminal.fg, 0.66) }}>
              Runtime and App depend only on the contract, so either side can be swapped independently.
            </Typography>
          </Box>
        </Box>

        <Box
          sx={{
            display: "flex",
            flexWrap: "wrap",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 1.5,
            pt: 2.5,
            fontFamily: (theme) => theme.fonts.mono,
            fontSize: "0.625rem",
            color: (theme) => alpha(theme.palette.terminal.fg, 0.62)
          }}
        >
          <span>© 2026 THE LEVIER AUTHORS</span>
          <span>CONTROL PLANE · RUNTIME · CLI</span>
          <Link href="#top" underline="none" sx={{ color: (theme) => alpha(theme.palette.terminal.fg, 0.72), "&:hover": { color: "terminal.accent" } }}>
            Back to top ↑
          </Link>
        </Box>
      </Container>
    </Box>
  );
}
