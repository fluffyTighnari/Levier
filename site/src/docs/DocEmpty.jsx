import { Box, Typography } from "@mui/material";
import { BookOpenText } from "@phosphor-icons/react";

export function DocEmpty({ copy }) {
  return (
    <Box
      sx={{
        maxWidth: 760,
        p: 4,
        border: 1,
        borderStyle: "dashed",
        borderColor: "divider",
        borderRadius: 1.25,
        textAlign: "center",
        color: "text.secondary"
      }}
    >
      <BookOpenText size={28} />
      <Typography sx={{ mt: 1.5, fontSize: "0.9375rem", fontWeight: 600, color: "text.primary" }}>
        {copy.empty.title}
      </Typography>
      <Typography sx={{ mt: 0.75, fontSize: "0.8125rem", lineHeight: 1.7 }}>
        {copy.empty.body}
      </Typography>
    </Box>
  );
}
