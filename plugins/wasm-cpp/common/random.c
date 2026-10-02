#include <unistd.h>
#include <openssl/rand.h>
#include <errno.h>

int getentropy(void *buffer, size_t len)
{
  RAND_bytes((uint8_t *)buffer, len);
  return 0;
}
