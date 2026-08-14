import { useState } from "react";
import { providerDot } from "../api";

interface ProviderLogoProps {
  id: string;
  /** Tailwind size classes for the square logo box, e.g. "size-11". */
  className?: string;
}

/**
 * Renders a provider brand logo inside a fixed square box. The image is
 * contained (never overflows, never distorts) regardless of the asset's
 * aspect ratio; on load failure it degrades to a brand-colored chip.
 */
export default function ProviderLogo({ id, className = "size-10" }: ProviderLogoProps) {
  const [failed, setFailed] = useState(false);

  return (
    <span className={`${className} grid shrink-0 place-items-center`}>
      {failed ? (
        <span
          className="h-full w-full rounded-lg"
          style={{ background: providerDot(id) }}
        />
      ) : (
        <img
          src={`/logos/${id}.svg`}
          alt=""
          aria-hidden="true"
          loading="lazy"
          onError={() => setFailed(true)}
          className="max-h-full max-w-full object-contain"
        />
      )}
    </span>
  );
}
