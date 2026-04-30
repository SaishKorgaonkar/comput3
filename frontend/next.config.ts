import type { NextConfig } from "next";

const BACKEND = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  output: "standalone",
  async rewrites() {
    return [
      {
        source: "/api/backend/:path*",
        destination: `${BACKEND}/:path*`,
      },
    ];
  },
};

export default nextConfig;
