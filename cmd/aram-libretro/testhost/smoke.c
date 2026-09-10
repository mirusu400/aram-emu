#include "../libretro.h"

#include <dlfcn.h>
#include <stdarg.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

struct core_api {
   void (*set_environment)(retro_environment_t);
   void (*set_video_refresh)(retro_video_refresh_t);
   void (*set_audio_sample)(retro_audio_sample_t);
   void (*set_audio_sample_batch)(retro_audio_sample_batch_t);
   void (*set_input_poll)(retro_input_poll_t);
   void (*set_input_state)(retro_input_state_t);
   void (*init)(void);
   void (*deinit)(void);
   unsigned (*api_version)(void);
   void (*get_system_info)(struct retro_system_info *);
   void (*get_system_av_info)(struct retro_system_av_info *);
   void (*reset)(void);
   void (*run)(void);
   size_t (*serialize_size)(void);
   bool (*serialize)(void *, size_t);
   bool (*unserialize)(const void *, size_t);
   bool (*load_game)(const struct retro_game_info *);
   void (*unload_game)(void);
};

static unsigned video_frames;
static unsigned input_polls;
static unsigned geometry_updates;
static unsigned video_width;
static unsigned video_height;
static size_t video_pitch;
static enum retro_pixel_format pixel_format;

static void frontend_log(enum retro_log_level level, const char *format, ...)
{
   va_list arguments;
   fprintf(stderr, "core[%d]: ", (int)level);
   va_start(arguments, format);
   vfprintf(stderr, format, arguments);
   va_end(arguments);
}

static bool environment(unsigned command, void *data)
{
   static const char save_directory[] = "/data/local/tmp/aram-libretro-save";
   switch (command) {
      case RETRO_ENVIRONMENT_SET_INPUT_DESCRIPTORS:
         return true;
      case RETRO_ENVIRONMENT_GET_LOG_INTERFACE:
         ((struct retro_log_callback *)data)->log = frontend_log;
         return true;
      case RETRO_ENVIRONMENT_GET_SAVE_DIRECTORY:
         *(const char **)data = save_directory;
         return true;
      case RETRO_ENVIRONMENT_SET_PIXEL_FORMAT:
         pixel_format = *(const enum retro_pixel_format *)data;
         return pixel_format == RETRO_PIXEL_FORMAT_XRGB8888;
      case RETRO_ENVIRONMENT_SET_GEOMETRY: {
         const struct retro_game_geometry *geometry = (const struct retro_game_geometry *)data;
         if (!geometry || !geometry->base_width || !geometry->base_height)
            return false;
         geometry_updates++;
         return true;
      }
      default:
         return false;
   }
}

static void video_refresh(const void *data, unsigned width, unsigned height, size_t pitch)
{
   if (!data || !width || !height || pitch < width * sizeof(uint32_t)) {
      fprintf(stderr, "invalid video callback\n");
      exit(20);
   }
   video_frames++;
   video_width = width;
   video_height = height;
   video_pitch = pitch;
}

static void audio_sample(int16_t left, int16_t right)
{
   (void)left;
   (void)right;
}

static size_t audio_batch(const int16_t *data, size_t frames)
{
   if (frames && !data) {
      fprintf(stderr, "invalid audio callback\n");
      exit(21);
   }
   return frames;
}

static void input_poll(void) { input_polls++; }

static int16_t input_state(unsigned port, unsigned device, unsigned index, unsigned id)
{
   (void)port;
   (void)index;
   if (device != RETRO_DEVICE_JOYPAD)
      return 0;
   return video_frames == 0 && id == RETRO_DEVICE_ID_JOYPAD_A ? 1 : 0;
}

static void write_u32le(uint8_t *output, uint32_t value)
{
   output[0] = (uint8_t)value;
   output[1] = (uint8_t)(value >> 8);
   output[2] = (uint8_t)(value >> 16);
   output[3] = (uint8_t)(value >> 24);
}

static size_t synthetic_eads(uint8_t *output, size_t capacity)
{
   const size_t offset = 0x80;
   const uint8_t code[] = {0x00, 0xb5, 0x00, 0xbe, 0xfe, 0xe7};
   const size_t size = offset + 0x30 + sizeof(code);
   if (capacity < size)
      return 0;
   memset(output, 0, size);
   memcpy(output + offset, "EADS", 4);
   write_u32le(output + offset + 4, 1);
   write_u32le(output + offset + 8, 1);
   write_u32le(output + offset + 12, 0x02000000);
   write_u32le(output + offset + 16, (uint32_t)sizeof(code));
   write_u32le(output + offset + 20, 0x03000000);
   write_u32le(output + offset + 24, 0x1000);
   memcpy(output + offset + 0x20, "SyntheticEADS", 13);
   memcpy(output + offset + 0x30, code, sizeof(code));
   return size;
}

static bool load_api(void *library, struct core_api *api)
{
#define LOAD(field, symbol) do { \
   *(void **)(&api->field) = dlsym(library, symbol); \
   if (!api->field) { fprintf(stderr, "missing %s: %s\n", symbol, dlerror()); return false; } \
} while (0)
   memset(api, 0, sizeof(*api));
   LOAD(set_environment, "retro_set_environment");
   LOAD(set_video_refresh, "retro_set_video_refresh");
   LOAD(set_audio_sample, "retro_set_audio_sample");
   LOAD(set_audio_sample_batch, "retro_set_audio_sample_batch");
   LOAD(set_input_poll, "retro_set_input_poll");
   LOAD(set_input_state, "retro_set_input_state");
   LOAD(init, "retro_init");
   LOAD(deinit, "retro_deinit");
   LOAD(api_version, "retro_api_version");
   LOAD(get_system_info, "retro_get_system_info");
   LOAD(get_system_av_info, "retro_get_system_av_info");
   LOAD(reset, "retro_reset");
   LOAD(run, "retro_run");
   LOAD(serialize_size, "retro_serialize_size");
   LOAD(serialize, "retro_serialize");
   LOAD(unserialize, "retro_unserialize");
   LOAD(load_game, "retro_load_game");
   LOAD(unload_game, "retro_unload_game");
#undef LOAD
   return true;
}

int main(int argc, char **argv)
{
   uint8_t content[0x200];
   uint8_t malformed[] = "not a WIPI image";
   size_t content_size;
   size_t state_size;
   void *state;
   void *library;
   struct core_api api;
   struct retro_system_info system_info;
   struct retro_system_av_info av_info;
   struct retro_game_info game;

   if (argc != 2) {
      fprintf(stderr, "usage: %s CORE.so\n", argv[0]);
      return 2;
   }
   library = dlopen(argv[1], RTLD_NOW | RTLD_LOCAL);
   if (!library) {
      fprintf(stderr, "dlopen failed: %s\n", dlerror());
      return 3;
   }
   if (!load_api(library, &api))
      return 4;
   if (api.api_version() != RETRO_API_VERSION) {
      fprintf(stderr, "libretro API version mismatch\n");
      return 5;
   }

   api.set_environment(environment);
   api.set_video_refresh(video_refresh);
   api.set_audio_sample(audio_sample);
   api.set_audio_sample_batch(audio_batch);
   api.set_input_poll(input_poll);
   api.set_input_state(input_state);
   api.init();
   api.get_system_info(&system_info);
   if (!system_info.library_name || strcmp(system_info.library_name, "ARAM") != 0 ||
       system_info.need_fullpath || !system_info.block_extract) {
      fprintf(stderr, "unexpected system info\n");
      return 6;
   }

   content_size = synthetic_eads(content, sizeof(content));
   memset(&game, 0, sizeof(game));
   game.path = "synthetic.dat";
   game.data = content;
   game.size = content_size;
   if (!api.load_game(&game)) {
      fprintf(stderr, "synthetic content load failed\n");
      return 7;
   }
   api.get_system_av_info(&av_info);
   if (av_info.geometry.base_width != 240 || av_info.geometry.base_height != 320 ||
       av_info.timing.fps != 62.5 || av_info.timing.sample_rate != 44100.0) {
      fprintf(stderr, "unexpected AV info: %ux%u %.3f %.1f\n",
         av_info.geometry.base_width, av_info.geometry.base_height,
         av_info.timing.fps, av_info.timing.sample_rate);
      return 8;
   }

   api.run();
   api.run();
   if (video_frames != 2 || input_polls != 2 || video_width != 240 || video_height != 320 ||
       video_pitch != 240 * sizeof(uint32_t) || pixel_format != RETRO_PIXEL_FORMAT_XRGB8888 ||
       geometry_updates == 0) {
      fprintf(stderr, "callback contract failed: video=%u input=%u geometry=%u size=%ux%u pitch=%zu format=%d\n",
         video_frames, input_polls, geometry_updates, video_width, video_height, video_pitch, (int)pixel_format);
      return 9;
   }

   state_size = api.serialize_size();
   if (state_size < 1024 || state_size > (128u << 20)) {
      fprintf(stderr, "unexpected state size: %zu\n", state_size);
      return 10;
   }
   state = malloc(state_size);
   if (!state || !api.serialize(state, state_size)) {
      fprintf(stderr, "serialize failed\n");
      return 11;
   }
   api.reset();
   if (!api.unserialize(state, state_size)) {
      fprintf(stderr, "unserialize failed\n");
      return 12;
   }
   free(state);
   api.run();
   api.unload_game();

   game.path = "broken.dat";
   game.data = malformed;
   game.size = sizeof(malformed) - 1;
   if (api.load_game(&game)) {
      fprintf(stderr, "malformed content was accepted\n");
      return 13;
   }

   api.deinit();
   dlclose(library);
   printf("PASS android-libretro api=%u frames=%u size=%ux%u state=%zu\n",
      RETRO_API_VERSION, video_frames, video_width, video_height, state_size);
   return 0;
}
