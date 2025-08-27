i/** @type {import('next').NextConfig} */
const nextConfig = {
  async rewrites() {
    return [
      {
        source: '/api/:path*',
        destination: 'http://localhost:4000/api/:path*',
      },
    ];
  },
  webpack: (config, { isServer }) => {
    // Ignore Windows system files to prevent watchpack errors
    if (!isServer) {
      const existingIgnored = config.watchOptions?.ignored;
      const ignoredArray = Array.isArray(existingIgnored) ? existingIgnored : [];

      config.watchOptions = {
        ...config.watchOptions,
        ignored: [
          "**/hiberfil.sys",
          "**/pagefile.sys",
          "**/swapfile.sys",
          ...ignoredArray,
        ],
      };
    }

    return config;
  },
};

module.exports = nextConfig;