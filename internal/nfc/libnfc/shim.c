//go:build libnfc && cgo

#include "shim.h"

#include <nfc/nfc.h>
#include <stdatomic.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>
#ifdef _WIN32
#include <windows.h>
#else
#include <time.h>
#endif

#define NFCX_MANUFACTURER_WRITE_TIMEOUT_MS 1000
#define NFCX_RF_RESET_DELAY_MS 20

struct nfcx_handle {
  nfc_context *context;
  nfc_device *device;
  nfc_target target;
  bool has_target;
  bool authenticated;
  uint8_t authenticated_sector;
};

static _Atomic size_t live_contexts = 0;
static _Atomic size_t live_devices = 0;
static _Atomic size_t live_handles = 0;

static int device_result(nfcx_handle *handle, int result, char *detail,
                         size_t detail_capacity, size_t *detail_length);

static void sleep_milliseconds(unsigned int milliseconds) {
#ifdef _WIN32
  Sleep(milliseconds);
#else
  struct timespec duration = {
      .tv_sec = milliseconds / 1000,
      .tv_nsec = (long)(milliseconds % 1000) * 1000000L,
  };
  (void)nanosleep(&duration, NULL);
#endif
}

static void set_detail(const char *source, char *output, size_t capacity,
                       size_t *output_length) {
  size_t length = source == NULL ? 0 : strlen(source);
  if (output_length != NULL) {
    *output_length = length;
  }
  if (output == NULL || capacity == 0) {
    return;
  }
  size_t copied = length;
  if (copied >= capacity) {
    copied = capacity - 1;
  }
  if (copied > 0) {
    memcpy(output, source, copied);
  }
  output[copied] = '\0';
}

int nfcx_runtime_version(char *output, size_t output_capacity,
                         size_t *output_length) {
  const char *version = nfc_version();
  if (version == NULL || output == NULL || output_capacity == 0) {
    if (output_length != NULL) {
      *output_length = 0;
    }
    return NFCX_ERR_INVALID_ARGUMENT;
  }
  size_t length = strlen(version);
  if (length >= output_capacity) {
    if (output_length != NULL) {
      *output_length = length;
    }
    output[0] = '\0';
    return NFCX_ERR_INTERNAL;
  }
  memcpy(output, version, length + 1);
  if (output_length != NULL) {
    *output_length = length;
  }
  return NFCX_OK;
}

int nfcx_configure_discovery_defaults(char *detail, size_t detail_capacity,
                                      size_t *detail_length) {
#ifdef _WIN32
  const char *names[] = {
      "LIBNFC_AUTO_SCAN",
      "LIBNFC_INTRUSIVE_SCAN",
  };
  for (size_t index = 0; index < sizeof(names) / sizeof(names[0]); ++index) {
    if (getenv(names[index]) != NULL) {
      continue;
    }
    if (_putenv_s(names[index], "true") != 0) {
      set_detail("failed to synchronize libnfc discovery environment", detail,
                 detail_capacity, detail_length);
      return NFCX_ERR_INTERNAL;
    }
    const char *configured = getenv(names[index]);
    if (configured == NULL || strcmp(configured, "true") != 0) {
      set_detail("libnfc discovery environment did not retain its defaults",
                 detail, detail_capacity, detail_length);
      return NFCX_ERR_INTERNAL;
    }
  }
#endif
  set_detail(NULL, detail, detail_capacity, detail_length);
  return NFCX_OK;
}

static int map_libnfc_error(int code) {
  switch (code) {
  case NFC_SUCCESS:
    return NFCX_OK;
  case NFC_ETIMEOUT:
    return NFCX_ERR_TIMEOUT;
  case NFC_EOPABORTED:
    return NFCX_ERR_CANCELED;
  case NFC_EMFCAUTHFAIL:
    return NFCX_ERR_AUTHENTICATION_FAILED;
  case NFC_ENOTSUCHDEV:
    return NFCX_ERR_DEVICE_DISCONNECTED;
  case NFC_ETGRELEASED:
    return NFCX_ERR_NO_CARD;
  case NFC_EINVARG:
    return NFCX_ERR_INVALID_ARGUMENT;
  case NFC_EDEVNOTSUPP:
  case NFC_ENOTIMPL:
    return NFCX_ERR_UNSUPPORTED;
  case NFC_EIO:
  case NFC_ERFTRANS:
  case NFC_ECHIP:
    return NFCX_ERR_IO;
  case NFC_EOVFLOW:
  case NFC_ESOFT:
  default:
    return NFCX_ERR_INTERNAL;
  }
}

static void clear_authentication(nfcx_handle *handle) {
  if (handle == NULL) {
    return;
  }
  handle->authenticated = false;
  handle->authenticated_sector = 0;
}

static void clear_target(nfcx_handle *handle) {
  if (handle == NULL) {
    return;
  }
  clear_authentication(handle);
  memset(&handle->target, 0, sizeof(handle->target));
  handle->has_target = false;
}

static bool same_target_identity(const nfc_target *left,
                                 const nfc_target *right) {
  if (left == NULL || right == NULL ||
      left->nti.nai.szUidLen != right->nti.nai.szUidLen ||
      left->nti.nai.btSak != right->nti.nai.btSak ||
      memcmp(left->nti.nai.abtAtqa, right->nti.nai.abtAtqa,
             NFCX_ATQA_LENGTH) != 0) {
    return false;
  }
  return memcmp(left->nti.nai.abtUid, right->nti.nai.abtUid,
                left->nti.nai.szUidLen) == 0;
}

static int classic_block_sector(const nfcx_handle *handle, uint8_t block,
                                uint8_t *sector) {
  if (handle == NULL || !handle->has_target || sector == NULL) {
    return NFCX_ERR_NO_CARD;
  }
  uint8_t sak = handle->target.nti.nai.btSak & (uint8_t)~0x04;
  if (sak == 0x08) {
    if (block >= 64) {
      return NFCX_ERR_INVALID_ARGUMENT;
    }
    *sector = block / 4;
    return NFCX_OK;
  }
  if (sak == 0x18) {
    *sector = block < 128 ? block / 4 : 32 + (block - 128) / 16;
    return NFCX_OK;
  }
  return NFCX_ERR_UNSUPPORTED;
}

static int require_authenticated_sector(nfcx_handle *handle, uint8_t block,
                                        uint8_t *sector, char *detail,
                                        size_t detail_capacity,
                                        size_t *detail_length) {
  if (!handle->has_target) {
    set_detail("select a card before a MIFARE Classic operation", detail,
               detail_capacity, detail_length);
    return NFCX_ERR_NO_CARD;
  }
  int code = classic_block_sector(handle, block, sector);
  if (code != NFCX_OK) {
    set_detail(code == NFCX_ERR_UNSUPPORTED
                   ? "selected card is not a supported MIFARE Classic 1K/4K card"
                   : "block is outside the selected card capacity",
               detail, detail_capacity, detail_length);
    return code;
  }
  if (!handle->authenticated || handle->authenticated_sector != *sector) {
    set_detail("target sector has not been authenticated", detail,
               detail_capacity, detail_length);
    return NFCX_ERR_NOT_AUTHENTICATED;
  }
  return NFCX_OK;
}

static int select_current_card(nfcx_handle *handle, nfc_target *target,
                               char *detail, size_t detail_capacity,
                               size_t *detail_length) {
  const nfc_modulation modulation = {
      .nmt = NMT_ISO14443A,
      .nbr = NBR_106,
  };
  memset(target, 0, sizeof(*target));
  int result = nfc_initiator_select_passive_target(
      handle->device, modulation, NULL, 0, target);
  if (result < 0) {
    return device_result(handle, result, detail, detail_capacity,
                         detail_length);
  }
  if (result == 0) {
    set_detail("no ISO14443A card present", detail, detail_capacity,
               detail_length);
    return NFCX_ERR_NO_CARD;
  }
  size_t uid_length = target->nti.nai.szUidLen;
  if (uid_length > NFCX_UID_MAX ||
      (uid_length != 4 && uid_length != 7 && uid_length != NFCX_UID_MAX)) {
    set_detail("libnfc returned an invalid ISO14443A UID length", detail,
               detail_capacity, detail_length);
    return NFCX_ERR_INTERNAL;
  }
  return NFCX_OK;
}

// When libnfc reports that the selected target disappeared, select once more
// to distinguish removal from replacement without parsing driver messages.
// Reselecting always expires Crypto1 authentication, even for the same card.
static int reselect_and_classify(nfcx_handle *handle, int same_card_code,
                                 char *detail, size_t detail_capacity,
                                 size_t *detail_length) {
  nfc_target expected = handle->target;
  clear_authentication(handle);

  nfc_target current;
  int code = select_current_card(handle, &current, detail, detail_capacity,
                                 detail_length);
  if (code != NFCX_OK) {
    clear_target(handle);
    return code;
  }
  handle->target = current;
  handle->has_target = true;
  if (!same_target_identity(&expected, &current)) {
    set_detail("card identity changed during the operation", detail,
               detail_capacity, detail_length);
    return NFCX_ERR_CARD_CHANGED;
  }
  if (same_card_code == NFCX_OK) {
    set_detail(NULL, detail, detail_capacity, detail_length);
  } else if (same_card_code == NFCX_ERR_AUTHENTICATION_FAILED) {
    set_detail("MIFARE Classic authentication failed", detail,
               detail_capacity, detail_length);
  } else if (same_card_code == NFCX_ERR_NOT_AUTHENTICATED) {
    set_detail("card was reselected and sector authentication expired", detail,
               detail_capacity, detail_length);
  }
  return same_card_code;
}

static int classify_command_failure(nfcx_handle *handle, int result,
                                    int same_card_code, char *detail,
                                    size_t detail_capacity,
                                    size_t *detail_length) {
  clear_authentication(handle);
  if (result == NFC_EOPABORTED || result == NFC_ENOTSUCHDEV) {
    return device_result(handle, result, detail, detail_capacity,
                         detail_length);
  }
  int presence = nfc_initiator_target_is_present(handle->device,
                                                 &handle->target);
  if (presence == NFC_SUCCESS) {
    if (same_card_code == NFCX_ERR_AUTHENTICATION_FAILED) {
      set_detail("MIFARE Classic authentication failed", detail,
                 detail_capacity, detail_length);
      return same_card_code;
    }
    return device_result(handle, result, detail, detail_capacity,
                         detail_length);
  }
  if (presence == NFC_EOPABORTED || presence == NFC_ENOTSUCHDEV) {
    return device_result(handle, presence, detail, detail_capacity,
                         detail_length);
  }
  return reselect_and_classify(handle, same_card_code, detail, detail_capacity,
                               detail_length);
}

static int device_result(nfcx_handle *handle, int result, char *detail,
                         size_t detail_capacity, size_t *detail_length) {
  if (result >= 0) {
    set_detail(NULL, detail, detail_capacity, detail_length);
    return NFCX_OK;
  }
  const char *message = NULL;
  if (handle != NULL && handle->device != NULL) {
    message = nfc_strerror(handle->device);
  }
  set_detail(message, detail, detail_capacity, detail_length);
  return map_libnfc_error(result);
}

static void destroy_handle(nfcx_handle *handle) {
  if (handle == NULL) {
    return;
  }
  if (handle->device != NULL) {
    nfc_close(handle->device);
    handle->device = NULL;
    atomic_fetch_sub(&live_devices, 1);
  }
  if (handle->context != NULL) {
    nfc_exit(handle->context);
    handle->context = NULL;
    atomic_fetch_sub(&live_contexts, 1);
  }
  free(handle);
  atomic_fetch_sub(&live_handles, 1);
}

int nfcx_list_devices(nfcx_device_info *devices, size_t capacity,
                      size_t *device_count, char *detail,
                      size_t detail_capacity, size_t *detail_length) {
  if (device_count == NULL || (capacity > 0 && devices == NULL)) {
    set_detail("invalid device output buffer", detail, detail_capacity,
               detail_length);
    return NFCX_ERR_INVALID_ARGUMENT;
  }
  *device_count = 0;

  int discovery_result = nfcx_configure_discovery_defaults(
      detail, detail_capacity, detail_length);
  if (discovery_result != NFCX_OK) {
    return discovery_result;
  }

  nfc_context *context = NULL;
  nfc_init(&context);
  if (context == NULL) {
    set_detail("libnfc failed to initialize", detail, detail_capacity,
               detail_length);
    return NFCX_ERR_INTERNAL;
  }
  atomic_fetch_add(&live_contexts, 1);

  nfc_connstring *connstrings = NULL;
  if (capacity > 0) {
    connstrings = calloc(capacity, sizeof(*connstrings));
    if (connstrings == NULL) {
      nfc_exit(context);
      atomic_fetch_sub(&live_contexts, 1);
      set_detail("failed to allocate device list", detail, detail_capacity,
                 detail_length);
      return NFCX_ERR_INTERNAL;
    }
  }

  size_t count = nfc_list_devices(context, connstrings, capacity);
  for (size_t index = 0; index < count; ++index) {
    const char *end = memchr(connstrings[index], '\0', NFCX_CONNSTRING_MAX);
    if (end == NULL) {
      free(connstrings);
      nfc_exit(context);
      atomic_fetch_sub(&live_contexts, 1);
      set_detail("libnfc returned an unterminated connstring", detail,
                 detail_capacity, detail_length);
      return NFCX_ERR_INTERNAL;
    }
    size_t length = (size_t)(end - connstrings[index]);
    memcpy(devices[index].connstring, connstrings[index], length);
    devices[index].connstring[length] = '\0';
    devices[index].connstring_length = length;
  }
  *device_count = count;

  free(connstrings);
  nfc_exit(context);
  atomic_fetch_sub(&live_contexts, 1);
  set_detail(NULL, detail, detail_capacity, detail_length);
  return NFCX_OK;
}

int nfcx_reader_open(const char *connstring, size_t connstring_length,
                     nfcx_handle **output, char *detail,
                     size_t detail_capacity, size_t *detail_length) {
  if (output == NULL) {
    set_detail("reader output is required", detail, detail_capacity,
               detail_length);
    return NFCX_ERR_INVALID_ARGUMENT;
  }
  *output = NULL;
  if (connstring == NULL || connstring_length == 0 ||
      connstring_length >= NFCX_CONNSTRING_MAX ||
      memchr(connstring, '\0', connstring_length) != NULL) {
    set_detail("invalid connstring", detail, detail_capacity, detail_length);
    return NFCX_ERR_INVALID_ARGUMENT;
  }

  nfcx_handle *handle = calloc(1, sizeof(*handle));
  if (handle == NULL) {
    set_detail("failed to allocate reader handle", detail, detail_capacity,
               detail_length);
    return NFCX_ERR_INTERNAL;
  }
  atomic_fetch_add(&live_handles, 1);

  nfc_init(&handle->context);
  if (handle->context == NULL) {
    set_detail("libnfc failed to initialize", detail, detail_capacity,
               detail_length);
    destroy_handle(handle);
    return NFCX_ERR_INTERNAL;
  }
  atomic_fetch_add(&live_contexts, 1);

  nfc_connstring native_connstring = {0};
  memcpy(native_connstring, connstring, connstring_length);
  handle->device = nfc_open(handle->context, native_connstring);
  if (handle->device == NULL) {
    set_detail("libnfc could not open the requested device", detail,
               detail_capacity, detail_length);
    destroy_handle(handle);
    return NFCX_ERR_DEVICE_DISCONNECTED;
  }
  atomic_fetch_add(&live_devices, 1);

  int result = nfc_initiator_init(handle->device);
  if (result < 0) {
    int code = device_result(handle, result, detail, detail_capacity,
                             detail_length);
    destroy_handle(handle);
    return code;
  }
  result = nfc_device_set_property_bool(handle->device, NP_INFINITE_SELECT,
                                        false);
  if (result < 0) {
    int code = device_result(handle, result, detail, detail_capacity,
                             detail_length);
    destroy_handle(handle);
    return code;
  }
  result = nfc_device_set_property_bool(handle->device, NP_EASY_FRAMING, true);
  if (result < 0) {
    int code = device_result(handle, result, detail, detail_capacity,
                             detail_length);
    destroy_handle(handle);
    return code;
  }

  *output = handle;
  set_detail(NULL, detail, detail_capacity, detail_length);
  return NFCX_OK;
}

int nfcx_reader_close(nfcx_handle **handle, char *detail,
                      size_t detail_capacity, size_t *detail_length) {
  if (handle == NULL || *handle == NULL) {
    set_detail(NULL, detail, detail_capacity, detail_length);
    return NFCX_OK;
  }
  destroy_handle(*handle);
  *handle = NULL;
  set_detail(NULL, detail, detail_capacity, detail_length);
  return NFCX_OK;
}

int nfcx_reader_name(nfcx_handle *handle, char *output,
                     size_t output_capacity, size_t *output_length,
                     char *detail, size_t detail_capacity,
                     size_t *detail_length) {
  if (output_length == NULL || output == NULL || output_capacity == 0) {
    set_detail("invalid device name output buffer", detail, detail_capacity,
               detail_length);
    return NFCX_ERR_INVALID_ARGUMENT;
  }
  *output_length = 0;
  output[0] = '\0';
  if (handle == NULL || handle->device == NULL) {
    set_detail("reader is not open", detail, detail_capacity, detail_length);
    return NFCX_ERR_NOT_OPEN;
  }
  const char *name = nfc_device_get_name(handle->device);
  if (name == NULL) {
    set_detail("libnfc did not provide a device name", detail, detail_capacity,
               detail_length);
    return NFCX_ERR_INTERNAL;
  }
  size_t length = strlen(name);
  if (length >= output_capacity) {
    set_detail("libnfc device name exceeds output buffer", detail,
               detail_capacity, detail_length);
    return NFCX_ERR_INTERNAL;
  }
  memcpy(output, name, length);
  output[length] = '\0';
  *output_length = length;
  set_detail(NULL, detail, detail_capacity, detail_length);
  return NFCX_OK;
}

int nfcx_reader_abort(nfcx_handle *handle, char *detail,
                      size_t detail_capacity, size_t *detail_length) {
  if (handle == NULL || handle->device == NULL) {
    set_detail("reader is not open", detail, detail_capacity, detail_length);
    return NFCX_ERR_NOT_OPEN;
  }
  int result = nfc_abort_command(handle->device);
  return device_result(handle, result, detail, detail_capacity, detail_length);
}

int nfcx_reader_card_info(nfcx_handle *handle, nfcx_card_info *output,
                         char *detail, size_t detail_capacity,
                         size_t *detail_length) {
  if (output != NULL) {
    memset(output, 0, sizeof(*output));
  }
  if (handle == NULL || handle->device == NULL) {
    set_detail("reader is not open", detail, detail_capacity, detail_length);
    return NFCX_ERR_NOT_OPEN;
  }
  if (output == NULL) {
    set_detail("card output is required", detail, detail_capacity,
               detail_length);
    return NFCX_ERR_INVALID_ARGUMENT;
  }

  nfc_target target = {0};
  if (handle->has_target) {
    // Repeating an unrestricted InListPassiveTarget against an already
    // selected PN532 UART target is unreliable on Windows and can surface as
    // an RF timeout. libnfc's presence check performs a bounded, UID-specific
    // reselect for Classic cards, which both verifies the same card and resets
    // Crypto1 without sending InRelease (which would HALT the card).
    clear_authentication(handle);
    int presence = nfc_initiator_target_is_present(handle->device,
                                                   &handle->target);
    if (presence == NFC_SUCCESS) {
      target = handle->target;
    } else {
      if (presence == NFC_EOPABORTED || presence == NFC_ENOTSUCHDEV) {
        return device_result(handle, presence, detail, detail_capacity,
                             detail_length);
      }
      // The saved target is gone or the backend cannot perform a presence
      // check. Forget it before doing one ordinary selection so a replacement
      // card can be discovered and reported to the caller.
      clear_target(handle);
    }
  }
  if (!handle->has_target) {
    int code = select_current_card(handle, &target, detail, detail_capacity,
                                   detail_length);
    if (code != NFCX_OK) {
      return code;
    }
    handle->target = target;
    handle->has_target = true;
  }

  size_t uid_length = target.nti.nai.szUidLen;
  memcpy(output->uid, target.nti.nai.abtUid, uid_length);
  output->uid_length = uid_length;
  memcpy(output->atqa, target.nti.nai.abtAtqa, NFCX_ATQA_LENGTH);
  output->atqa_length = NFCX_ATQA_LENGTH;
  output->sak = target.nti.nai.btSak;
  set_detail(NULL, detail, detail_capacity, detail_length);
  return NFCX_OK;
}

int nfcx_reader_authenticate(nfcx_handle *handle, uint8_t block,
                             uint8_t key_command, const uint8_t *key,
                             size_t key_length, char *detail,
                             size_t detail_capacity, size_t *detail_length) {
  if (handle == NULL || handle->device == NULL) {
    set_detail("reader is not open", detail, detail_capacity, detail_length);
    return NFCX_ERR_NOT_OPEN;
  }
  if (key == NULL || key_length != NFCX_KEY_LENGTH ||
      (key_command != NFCX_MIFARE_AUTH_A &&
       key_command != NFCX_MIFARE_AUTH_B)) {
    set_detail("invalid authentication arguments", detail, detail_capacity,
               detail_length);
    return NFCX_ERR_INVALID_ARGUMENT;
  }
  if (!handle->has_target) {
    set_detail("select a card before a MIFARE Classic operation", detail,
               detail_capacity, detail_length);
    return NFCX_ERR_NO_CARD;
  }
  uint8_t sector = 0;
  int code = classic_block_sector(handle, block, &sector);
  if (code != NFCX_OK) {
    set_detail(code == NFCX_ERR_UNSUPPORTED
                   ? "selected card is not a supported MIFARE Classic 1K/4K card"
                   : "block is outside the selected card capacity",
               detail, detail_capacity, detail_length);
    return code;
  }

  uint8_t command[2 + NFCX_KEY_LENGTH + 4] = {0};
  command[0] = key_command;
  command[1] = block;
  memcpy(command + 2, key, NFCX_KEY_LENGTH);
  size_t uid_length = handle->target.nti.nai.szUidLen;
  memcpy(command + 2 + NFCX_KEY_LENGTH,
         handle->target.nti.nai.abtUid + uid_length - 4, 4);

  clear_authentication(handle);
  uint8_t response[NFCX_BLOCK_LENGTH] = {0};
  int result = nfc_initiator_transceive_bytes(
      handle->device, command, sizeof(command), response, sizeof(response), -1);
  if (result < 0) {
    return classify_command_failure(handle, result,
                                    NFCX_ERR_AUTHENTICATION_FAILED, detail,
                                    detail_capacity, detail_length);
  }
  handle->authenticated = true;
  handle->authenticated_sector = sector;
  set_detail(NULL, detail, detail_capacity, detail_length);
  return NFCX_OK;
}

int nfcx_reader_read_block(nfcx_handle *handle, uint8_t block,
                           uint8_t *output, size_t output_capacity,
                           size_t *output_length, char *detail,
                           size_t detail_capacity, size_t *detail_length) {
  if (output_length != NULL) {
    *output_length = 0;
  }
  if (handle == NULL || handle->device == NULL) {
    set_detail("reader is not open", detail, detail_capacity, detail_length);
    return NFCX_ERR_NOT_OPEN;
  }
  if (output == NULL || output_length == NULL ||
      output_capacity < NFCX_BLOCK_LENGTH) {
    set_detail("invalid block output buffer", detail, detail_capacity,
               detail_length);
    return NFCX_ERR_INVALID_ARGUMENT;
  }
  uint8_t sector = 0;
  int code = require_authenticated_sector(handle, block, &sector, detail,
                                          detail_capacity, detail_length);
  if (code != NFCX_OK) {
    return code;
  }

  const uint8_t command[] = {NFCX_MIFARE_READ, block};
  uint8_t response[265] = {0};
  int result = nfc_initiator_transceive_bytes(
      handle->device, command, sizeof(command), response, sizeof(response), -1);
  if (result < 0) {
    return classify_command_failure(handle, result,
                                    NFCX_ERR_NOT_AUTHENTICATED, detail,
                                    detail_capacity, detail_length);
  }
  if (result != NFCX_BLOCK_LENGTH && result != NFCX_BLOCK_LENGTH + 2) {
    clear_authentication(handle);
    set_detail("MIFARE Classic read returned an unexpected block length", detail,
               detail_capacity, detail_length);
    return NFCX_ERR_IO;
  }
  memcpy(output, response, NFCX_BLOCK_LENGTH);
  *output_length = NFCX_BLOCK_LENGTH;
  set_detail(NULL, detail, detail_capacity, detail_length);
  return NFCX_OK;
}

int nfcx_reader_write_block(nfcx_handle *handle, uint8_t block,
                            const uint8_t *data, size_t data_length,
                            char *detail, size_t detail_capacity,
                            size_t *detail_length) {
  if (handle == NULL || handle->device == NULL) {
    set_detail("reader is not open", detail, detail_capacity, detail_length);
    return NFCX_ERR_NOT_OPEN;
  }
  if (data == NULL || data_length != NFCX_BLOCK_LENGTH) {
    set_detail("invalid block input buffer", detail, detail_capacity,
               detail_length);
    return NFCX_ERR_INVALID_ARGUMENT;
  }
  if (block == 0) {
    set_detail("ordinary writes to manufacturer block 0 are forbidden", detail,
               detail_capacity, detail_length);
    return NFCX_ERR_INVALID_ARGUMENT;
  }
  uint8_t sector = 0;
  int code = require_authenticated_sector(handle, block, &sector, detail,
                                          detail_capacity, detail_length);
  if (code != NFCX_OK) {
    return code;
  }

  uint8_t command[2 + NFCX_BLOCK_LENGTH] = {0};
  command[0] = NFCX_MIFARE_WRITE;
  command[1] = block;
  memcpy(command + 2, data, NFCX_BLOCK_LENGTH);
  uint8_t response[265] = {0};
  int result = nfc_initiator_transceive_bytes(
      handle->device, command, sizeof(command), response, sizeof(response), -1);
  if (result < 0) {
    return classify_command_failure(handle, result,
                                    NFCX_ERR_NOT_AUTHENTICATED, detail,
                                    detail_capacity, detail_length);
  }
  set_detail(NULL, detail, detail_capacity, detail_length);
  return NFCX_OK;
}

int nfcx_reader_write_manufacturer_block(
    nfcx_handle *handle, const uint8_t *data, size_t data_length, char *detail,
    size_t detail_capacity, size_t *detail_length) {
  if (handle == NULL || handle->device == NULL) {
    set_detail("reader is not open", detail, detail_capacity, detail_length);
    return NFCX_ERR_NOT_OPEN;
  }
  if (data == NULL || data_length != NFCX_BLOCK_LENGTH) {
    set_detail("invalid manufacturer block input buffer", detail,
               detail_capacity, detail_length);
    return NFCX_ERR_INVALID_ARGUMENT;
  }
  uint8_t sector = 0;
  int code = require_authenticated_sector(handle, 0, &sector, detail,
                                          detail_capacity, detail_length);
  if (code != NFCX_OK) {
    return code;
  }

  uint8_t command[2 + NFCX_BLOCK_LENGTH] = {0};
  command[0] = NFCX_MIFARE_WRITE;
  command[1] = 0;
  memcpy(command + 2, data, NFCX_BLOCK_LENGTH);
  uint8_t response[265] = {0};
  int result = nfc_initiator_transceive_bytes(
      handle->device, command, sizeof(command), response, sizeof(response),
      NFCX_MANUFACTURER_WRITE_TIMEOUT_MS);
  if (result == NFC_EOPABORTED || result == NFC_ENOTSUCHDEV) {
    return device_result(handle, result, detail, detail_capacity,
                         detail_length);
  }

  // Changing block 0 can invalidate the selected target before PN53x returns
  // the final response. Preserve that response error, but always power-cycle
  // the RF field so the PICC reloads its anti-collision UID. The Go workflow
  // treats RF/timeout/I/O results as indeterminate and decides success only by
  // selecting and fully reading back the requested new identity.
  char write_error[256] = {0};
  if (result < 0) {
    const char *message = nfc_strerror(handle->device);
    if (message != NULL) {
      size_t length = strlen(message);
      if (length >= sizeof(write_error)) {
        length = sizeof(write_error) - 1;
      }
      memcpy(write_error, message, length);
      write_error[length] = '\0';
    }
  }
  clear_target(handle);
  int field_result =
      nfc_device_set_property_bool(handle->device, NP_ACTIVATE_FIELD, false);
  if (field_result < 0) {
    return device_result(handle, field_result, detail, detail_capacity,
                         detail_length);
  }
  sleep_milliseconds(NFCX_RF_RESET_DELAY_MS);
  field_result =
      nfc_device_set_property_bool(handle->device, NP_ACTIVATE_FIELD, true);
  if (field_result < 0) {
    return device_result(handle, field_result, detail, detail_capacity,
                         detail_length);
  }
  sleep_milliseconds(NFCX_RF_RESET_DELAY_MS);
  if (result < 0) {
    set_detail(write_error[0] == '\0' ? "manufacturer write response failed"
                                      : write_error,
               detail, detail_capacity, detail_length);
    return map_libnfc_error(result);
  }
  set_detail(NULL, detail, detail_capacity, detail_length);
  return NFCX_OK;
}

int nfcx_smoke(char *detail, size_t detail_capacity, size_t *detail_length) {
  nfc_context *context = NULL;
  nfc_init(&context);
  if (context == NULL) {
    set_detail("libnfc failed to initialize", detail, detail_capacity,
               detail_length);
    return NFCX_ERR_INTERNAL;
  }
  atomic_fetch_add(&live_contexts, 1);
  nfc_exit(context);
  atomic_fetch_sub(&live_contexts, 1);
  set_detail(NULL, detail, detail_capacity, detail_length);
  return NFCX_OK;
}

void nfcx_get_resource_count(nfcx_resource_count *output) {
  if (output == NULL) {
    return;
  }
  output->contexts = atomic_load(&live_contexts);
  output->devices = atomic_load(&live_devices);
  output->handles = atomic_load(&live_handles);
}
