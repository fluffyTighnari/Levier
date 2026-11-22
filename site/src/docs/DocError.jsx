import { Alert, AlertTitle, Box, Button, Typography } from "@mui/material";
import { Warning } from "@phosphor-icons/react";

export function DocError({
  item,
  error,
  onRetry,
  title,
  fileLabel,
  hint,
  copy
}) {
  const message = error && error.message ? error.message : String(error || "Unknown error");
  const isFile = window.location.protocol === "file:";
  const file = fileLabel || (item ? item.file : "the document");
  const heading = title || copy.error.pageTitle;

  return (
    <Alert
      severity="error"
      icon={<Warning size={20} />}
      sx={{ maxWidth: 760, alignItems: "flex-start" }}
      action={
        <Button color="inherit" size="small" onClick={onRetry} sx={{ minHeight: 30, px: 1.25 }}>
          {copy.error.retry}
        </Button>
      }
    >
      <AlertTitle sx={{ fontSize: "0.875rem", fontWeight: 600 }}>{heading}</AlertTitle>
      <Typography sx={{ fontSize: "0.8125rem", lineHeight: 1.7 }}>
        {copy.error.pageIntro}{" "}
        <Box component="code" sx={{ px: 0.5, py: 0.1, borderRadius: 0.5, bgcolor: "action.hover", fontFamily: (theme) => theme.fonts.mono, fontSize: "0.75rem" }}>{file}</Box>.
      </Typography>
      <Typography sx={{ mt: 1, fontFamily: (theme) => theme.fonts.mono, fontSize: "0.6875rem", color: "text.secondary" }}>
        {message}
      </Typography>
      <Typography sx={{ mt: 1.25, fontSize: "0.75rem", lineHeight: 1.7, color: "text.secondary" }}>
        {hint || (isFile ? copy.error.fileHint : copy.error.pathHint)}
      </Typography>
    </Alert>
  );
}
