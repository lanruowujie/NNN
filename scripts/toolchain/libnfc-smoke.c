#include <nfc/nfc.h>

#include <stdio.h>
#include <stdlib.h>

int
main(void)
{
  nfc_context *context = NULL;
  nfc_connstring devices[8];

  nfc_init(&context);
  if (context == NULL) {
    fputs("libnfc smoke: nfc_init returned no context\n", stderr);
    return 1;
  }

  const size_t count = nfc_list_devices(context, devices, 8);
  printf("libnfc smoke: version=%s devices=%zu\n", nfc_version(), count);
  nfc_exit(context);

  const char *expected_text = getenv("NFCX_EXPECT_DEVICE_COUNT");
  if (expected_text != NULL) {
    const size_t expected = (size_t)strtoul(expected_text, NULL, 10);
    if (count != expected) {
      fprintf(stderr, "libnfc smoke: expected %zu devices, got %zu\n", expected, count);
      return 1;
    }
  }
  return 0;
}
