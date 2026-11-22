import { Box, Drawer, IconButton, Typography } from "@mui/material";
import { X } from "@phosphor-icons/react";

export function MobileDrawer({ open, onClose, title, children, footer }) {
  return (
    <Drawer
      anchor="left"
      open={open}
      onClose={onClose}
      slotProps={{
        paper: {
          sx: {
            width: 310,
            maxWidth: "86vw",
            borderRight: 1,
            borderColor: "text.primary",
            bgcolor: "background.default"
          }
        }
      }}
    >
      <Box component="nav" aria-label={title} sx={{ p: 2.25, pb: 4 }}>
        <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 2, pb: 1.5, borderBottom: 1, borderColor: "text.primary" }}>
          <Typography sx={{ fontSize: "0.875rem", fontWeight: 600, color: "text.primary" }}>{title}</Typography>
          <IconButton onClick={() => onClose(null, "closeButton")} aria-label="Close navigation" size="small">
            <X size={18} />
          </IconButton>
        </Box>
        {children}
        {footer ? <Box sx={{ mt: 3, pt: 2, borderTop: 1, borderColor: "divider" }}>{footer}</Box> : null}
      </Box>
    </Drawer>
  );
}
