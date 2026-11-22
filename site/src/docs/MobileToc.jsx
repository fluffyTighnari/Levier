import { Accordion, AccordionDetails, AccordionSummary, Box, Typography } from "@mui/material";
import { CaretDown } from "@phosphor-icons/react";
import { TocList } from "./DocToc";

export function MobileToc({ headings, activeId, slug, onNavigate, expanded, onChange, copy }) {
  if (!headings || headings.length === 0) return null;
  return (
    <Accordion
      expanded={expanded}
      onChange={onChange}
      slotProps={{ heading: { component: "div" } }}
      sx={{ mt: 2.25, mb: 4, display: { lg: "none" } }}
    >
      <AccordionSummary component="div" expandIcon={<CaretDown size={16} />} aria-controls="mobile-toc-content">
        <Box sx={{ display: "flex", alignItems: "baseline", gap: 2 }}>
          <Typography sx={{ fontSize: "0.75rem", fontWeight: 600, color: "text.primary" }}>{copy.toc.title}</Typography>
          <Typography sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.59375rem", color: "text.secondary" }}>
            {copy.toc.count(headings.length)}
          </Typography>
        </Box>
      </AccordionSummary>
      <AccordionDetails id="mobile-toc-content">
        <TocList
          headings={headings}
          activeId={activeId}
          slug={slug}
          onNavigate={(id) => {
            onChange(null, false);
            window.setTimeout(() => onNavigate(id), 140);
          }}
        />
      </AccordionDetails>
    </Accordion>
  );
}
