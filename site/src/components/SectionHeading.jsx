import { Box, Typography } from "@mui/material";

export function SectionHeading({ index, title, note, id }) {
  return (
    <Box
      component="header"
      id={id}
      sx={{
        display: "grid",
        gridTemplateColumns: { xs: "32px minmax(0, 1fr)", md: "42px minmax(0, 1fr) auto" },
        alignItems: "baseline",
        gap: 1.75,
        pb: 1.5,
        mb: { xs: 4, md: 6 },
        borderBottom: 1,
        borderColor: "text.primary"
      }}
    >
      <Typography component="span" sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.71875rem", color: "primary.main" }}>
        {index}
      </Typography>
      <Typography variant="h2" component="h2" sx={{ fontSize: { xs: "1.5rem", md: "1.85rem" } }}>
        {title}
      </Typography>
      {note ? (
        <Typography
          component="span"
          sx={{ display: { xs: "none", md: "block" }, fontSize: "0.71875rem", color: "text.secondary", textAlign: "right" }}
        >
          {note}
        </Typography>
      ) : null}
    </Box>
  );
}
