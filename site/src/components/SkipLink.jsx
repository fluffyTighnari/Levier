import { Button } from "@mui/material";

export function SkipLink({ href = "#main", children = "Skip to main content" }) {
  return (
    <Button
      component="a"
      href={href}
      variant="text"
      sx={{
        position: "fixed",
        top: -80,
        left: 16,
        zIndex: 2200,
        bgcolor: "text.primary",
        color: "background.paper",
        borderRadius: 1,
        px: 2,
        py: 1,
        "&:focus": {
          top: 12,
          bgcolor: "text.primary",
          color: "background.paper"
        }
      }}
    >
      {children}
    </Button>
  );
}
