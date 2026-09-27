#ifndef NFCX_LIBNFC_SHIM_H
#define NFCX_LIBNFC_SHIM_H

#include <stddef.h>
#include <stdint.h>

#define NFCX_CONNSTRING_MAX 1024
#define NFCX_UID_MAX 10
#define NFCX_ATQA_LENGTH 2
#define NFCX_KEY_LENGTH 6
#define NFCX_BLOCK_LENGTH 16
#define NFCX_DEVICE_NAME_MAX 256

typedef enum {
  NFCX_OK = 0,
  NFCX_ERR_NO_CARD = 1,
  NFCX_ERR_TIMEOUT = 2,
  NFCX_ERR_AUTHENTICATION_FAILED = 3,
  NFCX_ERR_DEVICE_DISCONNECTED = 4,
  NFCX_ERR_IO = 5,
  NFCX_ERR_CANCELED = 6,
  NFCX_ERR_INVALID_ARGUMENT = 7,
  NFCX_ERR_NOT_OPEN = 8,
  NFCX_ERR_UNSUPPORTED = 9,
  NFCX_ERR_BUSY = 10,
  NFCX_ERR_INTERNAL = 11,
  NFCX_ERR_PERMISSION = 12,
  NFCX_ERR_CARD_CHANGED = 13,
  NFCX_ERR_NOT_AUTHENTICATED = 14,
  NFCX_ERR_VERIFICATION_FAILED = 15
} nfcx_error_code;

typedef enum {
  NFCX_MIFARE_AUTH_A = 0x60,
  NFCX_MIFARE_AUTH_B = 0x61,
  NFCX_MIFARE_READ = 0x30,
  NFCX_MIFARE_WRITE = 0xA0
} nfcx_mifare_command;

typedef struct nfcx_handle nfcx_handle;

typedef struct {
  char connstring[NFCX_CONNSTRING_MAX];
  size_t connstring_length;
} nfcx_device_info;

typedef struct {
  uint8_t uid[NFCX_UID_MAX];
  size_t uid_length;
  uint8_t atqa[NFCX_ATQA_LENGTH];
  size_t atqa_length;
  uint8_t sak;
} nfcx_card_info;

typedef struct {
  size_t contexts;
  size_t devices;
  size_t handles;
} nfcx_resource_count;

// Copies the linked libnfc version without exposing a library-owned pointer.
int nfcx_runtime_version(char *output, size_t output_capacity,
                         size_t *output_length);

// Synchronizes NFCX's discovery defaults with the C runtime environment used
// by libnfc. This is required on Windows, where Go and the CRT keep separate
// environment tables after process startup.
int nfcx_configure_discovery_defaults(char *detail, size_t detail_capacity,
                                      size_t *detail_length);

// Enumeration owns its temporary libnfc context. It does not retain any caller
// buffer after returning.
int nfcx_list_devices(nfcx_device_info *devices, size_t capacity,
                      size_t *device_count, char *detail,
                      size_t detail_capacity, size_t *detail_length);

// A successful open transfers one opaque handle to the caller. The handle owns
// both nfc_context and nfc_device and must eventually be passed to close. On
// failure output is NULL and all partially created resources have been freed.
int nfcx_reader_open(const char *connstring, size_t connstring_length,
                     nfcx_handle **output, char *detail,
                     size_t detail_capacity, size_t *detail_length);

// The friendly name belongs to libnfc. This function copies it into caller
// storage so no native pointer crosses the shim boundary.
int nfcx_reader_name(nfcx_handle *handle, char *output,
                     size_t output_capacity, size_t *output_length,
                     char *detail, size_t detail_capacity,
                     size_t *detail_length);

// Close releases and NULLs the caller's handle. Passing a NULL handle is a
// successful no-op.
int nfcx_reader_close(nfcx_handle **handle, char *detail,
                      size_t detail_capacity, size_t *detail_length);

// Abort does not take ownership of the handle. It is the only shim call allowed
// concurrently with an active reader command; the caller must keep the handle
// alive until both calls return.
int nfcx_reader_abort(nfcx_handle *handle, char *detail,
                      size_t detail_capacity, size_t *detail_length);

int nfcx_reader_card_info(nfcx_handle *handle, nfcx_card_info *output,
                         char *detail, size_t detail_capacity,
                         size_t *detail_length);

int nfcx_reader_authenticate(nfcx_handle *handle, uint8_t block,
                             uint8_t command, const uint8_t *key,
                             size_t key_length, char *detail,
                             size_t detail_capacity, size_t *detail_length);

int nfcx_reader_read_block(nfcx_handle *handle, uint8_t block,
                           uint8_t *output, size_t output_capacity,
                           size_t *output_length, char *detail,
                           size_t detail_capacity, size_t *detail_length);

int nfcx_reader_write_block(nfcx_handle *handle, uint8_t block,
                            const uint8_t *data, size_t data_length,
                            char *detail, size_t detail_capacity,
                            size_t *detail_length);

// This is deliberately separate from the ordinary block writer. It always
// targets block 0 and is reachable only through NFCX's confirmed CUID/Gen2
// workflow after sector-0 authentication.
int nfcx_reader_write_manufacturer_block(
    nfcx_handle *handle, const uint8_t *data, size_t data_length, char *detail,
    size_t detail_capacity, size_t *detail_length);

int nfcx_smoke(char *detail, size_t detail_capacity, size_t *detail_length);
void nfcx_get_resource_count(nfcx_resource_count *output);

#endif
