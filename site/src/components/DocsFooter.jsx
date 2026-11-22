import { Box, Container, Link, Typography } from "@mui/material";

export function DocsFooter({ copy }) {
  return (
    <Box component="footer" sx={{ borderTop: 1, borderColor: "divider", mt: 6 }}>
      <Container maxWidth="lg">
        <Box
          sx={{
            display: "flex",
            flexWrap: "wrap",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 1,
            py: 2.25,
            fontSize: "0.71875rem",
            color: "text.secondary"
          }}
        >
          <Typography component="p" variant="caption" sx={{ color: "text.secondary" }}>
            {copy.footer.label}
          </Typography>
          <Box sx={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 1 }}>
            <Link href="../index.html" underline="hover">{copy.footer.home}</Link>
            <Box component="span" aria-hidden="true">·</Box>
            <Link href="https://github.com/fluffyTighnari/levier" target="_blank" rel="noopener noreferrer" underline="hover">
              GitHub ↗
            </Link>
            <Box component="span" aria-hidden="true">·</Box>
            <Link href="https://github.com/fluffyTighnari/levier/blob/main/docs/SCOPE.md" target="_blank" rel="noopener noreferrer" underline="hover">
              SCOPE ↗
            </Link>
          </Box>
        </Box>
      </Container>
    </Box>
  );
}
