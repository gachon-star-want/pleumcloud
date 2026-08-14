import { useState } from "react";
import { providerDot } from "../api";

interface ProviderLogoProps {
  id: string;
  /** Tailwind height class for the logo (width scales to fit). */
  className?: string;
}

/** Renders the provider's brand logo from /logos/<id>.svg, falling back to a
 * colored dot if the asset fails to load. */
export default function ProviderLogo({ id, className = "h-8" }: ProviderLogoProps) {
  const [failed, setFailed] = useState(false);

  if (failed) {
    return (
      <span
        className={`inline-block ${className} aspect-square rounded-lg`}
        style={{ background: providerDot(id) }}
      />
    );
  }
  return (
    <img
      src={`/logos/${id}.svg`}
      alt=""
      loading="lazy"
      onError={() => setFailed(true)}
      className={`${className} w-auto max-w-[96px] object-contain`}
    />
  );
}
