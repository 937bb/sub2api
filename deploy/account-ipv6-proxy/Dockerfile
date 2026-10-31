FROM sub2api-b:official-019-933aaad77
COPY account-ipv6-proxy /app/account-ipv6-proxy
USER 65532:65532
ENTRYPOINT ["/app/account-ipv6-proxy"]
CMD ["/config/config.json"]
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD wget -q -O /dev/null http://127.0.0.1:26443/health || exit 1
