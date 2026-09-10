#include "libretro.h"
#include "_cgo_export.h"

#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static retro_environment_t environment_cb;
static retro_video_refresh_t video_cb;
static retro_audio_sample_t audio_cb;
static retro_audio_sample_batch_t audio_batch_cb;
static retro_input_poll_t input_poll_cb;
static retro_input_state_t input_state_cb;
static retro_log_printf_t log_cb;

static uint32_t *video_buffer;
static size_t video_capacity;
static int16_t *audio_buffer;
static size_t audio_capacity;
static unsigned last_width;
static unsigned last_height;
static bool can_dupe;

static void log_message(enum retro_log_level level, const char *message)
{
   if (log_cb)
      log_cb(level, "%s\n", message);
}

static void log_last_error(const char *operation)
{
   char error[1024];
   char message[1152];
   aram_copy_last_error(error, sizeof(error));
   snprintf(message, sizeof(message), "%s: %s", operation, error[0] ? error : "unknown error");
   log_message(RETRO_LOG_ERROR, message);
}

static bool reserve_video(size_t pixels)
{
   uint32_t *replacement;
   if (pixels <= video_capacity)
      return true;
   replacement = (uint32_t *)realloc(video_buffer, pixels * sizeof(*video_buffer));
   if (!replacement)
      return false;
   video_buffer = replacement;
   video_capacity = pixels;
   return true;
}

static bool reserve_audio(size_t samples)
{
   int16_t *replacement;
   if (samples <= audio_capacity)
      return true;
   replacement = (int16_t *)realloc(audio_buffer, samples * sizeof(*audio_buffer));
   if (!replacement)
      return false;
   audio_buffer = replacement;
   audio_capacity = samples;
   return true;
}

static void publish_geometry(unsigned width, unsigned height)
{
   struct retro_game_geometry geometry;
   if (!environment_cb || !width || !height || (width == last_width && height == last_height))
      return;
   memset(&geometry, 0, sizeof(geometry));
   geometry.base_width = width;
   geometry.base_height = height;
   geometry.max_width = aram_av_max_width();
   geometry.max_height = aram_av_max_height();
   geometry.aspect_ratio = (float)width / (float)height;
   environment_cb(RETRO_ENVIRONMENT_SET_GEOMETRY, &geometry);
   last_width = width;
   last_height = height;
}

void retro_set_environment(retro_environment_t cb)
{
   static const struct retro_input_descriptor descriptors[] = {
      {0, RETRO_DEVICE_JOYPAD, 0, RETRO_DEVICE_ID_JOYPAD_UP, "Keypad Up"},
      {0, RETRO_DEVICE_JOYPAD, 0, RETRO_DEVICE_ID_JOYPAD_DOWN, "Keypad Down"},
      {0, RETRO_DEVICE_JOYPAD, 0, RETRO_DEVICE_ID_JOYPAD_LEFT, "Keypad Left"},
      {0, RETRO_DEVICE_JOYPAD, 0, RETRO_DEVICE_ID_JOYPAD_RIGHT, "Keypad Right"},
      {0, RETRO_DEVICE_JOYPAD, 0, RETRO_DEVICE_ID_JOYPAD_A, "Select / OK"},
      {0, RETRO_DEVICE_JOYPAD, 0, RETRO_DEVICE_ID_JOYPAD_B, "Back / Clear"},
      {0, RETRO_DEVICE_JOYPAD, 0, RETRO_DEVICE_ID_JOYPAD_X, "Left Soft Key"},
      {0, RETRO_DEVICE_JOYPAD, 0, RETRO_DEVICE_ID_JOYPAD_Y, "Right Soft Key"},
      {0, RETRO_DEVICE_JOYPAD, 0, RETRO_DEVICE_ID_JOYPAD_START, "Menu"},
      {0, RETRO_DEVICE_JOYPAD, 0, RETRO_DEVICE_ID_JOYPAD_SELECT, "Hold for Numeric Keypad"},
      {0, RETRO_DEVICE_JOYPAD, 0, RETRO_DEVICE_ID_JOYPAD_L, "Star / Numeric 7"},
      {0, RETRO_DEVICE_JOYPAD, 0, RETRO_DEVICE_ID_JOYPAD_R, "Hash / Numeric 9"},
      {0, RETRO_DEVICE_JOYPAD, 0, RETRO_DEVICE_ID_JOYPAD_L2, "Send / Numeric Star"},
      {0, RETRO_DEVICE_JOYPAD, 0, RETRO_DEVICE_ID_JOYPAD_R2, "End / Numeric Hash"},
      {0, 0, 0, 0, NULL},
   };
   struct retro_log_callback logging;
   environment_cb = cb;
   if (!cb)
      return;
   cb(RETRO_ENVIRONMENT_SET_INPUT_DESCRIPTORS, (void *)descriptors);
   if (cb(RETRO_ENVIRONMENT_GET_LOG_INTERFACE, &logging))
      log_cb = logging.log;
   if (!cb(RETRO_ENVIRONMENT_GET_CAN_DUPE, &can_dupe))
      can_dupe = false;
}

void retro_set_video_refresh(retro_video_refresh_t cb) { video_cb = cb; }
void retro_set_audio_sample(retro_audio_sample_t cb) { audio_cb = cb; }
void retro_set_audio_sample_batch(retro_audio_sample_batch_t cb) { audio_batch_cb = cb; }
void retro_set_input_poll(retro_input_poll_t cb) { input_poll_cb = cb; }
void retro_set_input_state(retro_input_state_t cb) { input_state_cb = cb; }

void retro_init(void)
{
   const char *save_directory = NULL;
   aram_initialize();
   if (environment_cb && environment_cb(RETRO_ENVIRONMENT_GET_SAVE_DIRECTORY, &save_directory) && save_directory)
      aram_set_save_directory((char *)save_directory);
}

void retro_deinit(void)
{
   aram_deinitialize();
   free(video_buffer);
   free(audio_buffer);
   video_buffer = NULL;
   audio_buffer = NULL;
   video_capacity = 0;
   audio_capacity = 0;
   last_width = 0;
   last_height = 0;
}

unsigned retro_api_version(void) { return RETRO_API_VERSION; }

void retro_get_system_info(struct retro_system_info *info)
{
   memset(info, 0, sizeof(*info));
   info->library_name = "ARAM";
   info->library_version = "0.1.0";
   info->valid_extensions = "dat|jar|zip|elf|bin|wbin|wbt|sgs";
   info->need_fullpath = false;
   /* JAR/ZIP containers are inputs themselves, not archives for the frontend
    * to unpack before loading. */
   info->block_extract = true;
}

void retro_get_system_av_info(struct retro_system_av_info *info)
{
   unsigned width = aram_av_width();
   unsigned height = aram_av_height();
   memset(info, 0, sizeof(*info));
   info->geometry.base_width = width;
   info->geometry.base_height = height;
   info->geometry.max_width = aram_av_max_width();
   info->geometry.max_height = aram_av_max_height();
   info->geometry.aspect_ratio = height ? (float)width / (float)height : 0.0f;
   info->timing.fps = aram_av_fps();
   info->timing.sample_rate = aram_av_sample_rate();
}

void retro_set_controller_port_device(unsigned port, unsigned device)
{
   (void)port;
   (void)device;
}

void retro_reset(void)
{
   if (!aram_reset())
      log_last_error("reset");
}

void retro_run(void)
{
   uint16_t buttons = 0;
   size_t pixels;
   size_t samples;
   size_t frames;
   unsigned width;
   unsigned height;
   unsigned id;

   if (input_poll_cb)
      input_poll_cb();
   if (input_state_cb)
      for (id = 0; id < 16; id++)
         if (input_state_cb(0, RETRO_DEVICE_JOYPAD, 0, id))
            buttons |= (uint16_t)(1u << id);

   if (!aram_run(buttons)) {
      log_last_error("run");
      /* A failed frame still counts as a frame: dupe the previous one so the
       * frontend keeps its timing instead of stalling on a missing refresh. */
      if (video_cb && can_dupe && last_width && last_height)
         video_cb(NULL, last_width, last_height, 0);
      return;
   }

   width = aram_frame_width();
   height = aram_frame_height();
   pixels = aram_video_pixels();
   if (pixels && reserve_video(pixels) && aram_copy_video(video_buffer, video_capacity)) {
      publish_geometry(width, height);
      if (video_cb)
         video_cb(video_buffer, width, height, aram_frame_pitch());
   }

   samples = aram_audio_samples();
   frames = aram_audio_frames();
   if (samples && reserve_audio(samples) && aram_copy_audio(audio_buffer, audio_capacity)) {
      if (audio_batch_cb)
         audio_batch_cb(audio_buffer, frames);
      else if (audio_cb) {
         size_t frame;
         for (frame = 0; frame < frames; frame++)
            audio_cb(audio_buffer[frame * 2], audio_buffer[frame * 2 + 1]);
      }
   }
}

size_t retro_serialize_size(void) { return aram_serialize_size(); }

bool retro_serialize(void *data, size_t size)
{
   if (!aram_serialize(data, size)) {
      log_last_error("serialize");
      return false;
   }
   return true;
}

bool retro_unserialize(const void *data, size_t size)
{
   if (!aram_unserialize((void *)data, size)) {
      log_last_error("unserialize");
      return false;
   }
   return true;
}

void retro_cheat_reset(void) {}

void retro_cheat_set(unsigned index, bool enabled, const char *code)
{
   (void)index;
   (void)enabled;
   (void)code;
}

bool retro_load_game(const struct retro_game_info *game)
{
   enum retro_pixel_format format = RETRO_PIXEL_FORMAT_XRGB8888;
   const char *name;
   if (!game || !game->data || !game->size)
      return false;
   if (!environment_cb || !environment_cb(RETRO_ENVIRONMENT_SET_PIXEL_FORMAT, &format)) {
      log_message(RETRO_LOG_ERROR, "frontend rejected XRGB8888 video");
      return false;
   }
   name = game->path ? game->path : "content";
   if (!aram_load_game((char *)name, (void *)game->data, game->size)) {
      log_last_error("load game");
      return false;
   }
   last_width = 0;
   last_height = 0;
   publish_geometry(aram_av_width(), aram_av_height());
   return true;
}

bool retro_load_game_special(unsigned type, const struct retro_game_info *info, size_t count)
{
   (void)type;
   (void)info;
   (void)count;
   return false;
}

void retro_unload_game(void)
{
   if (!aram_unload_game())
      log_last_error("unload game");
}

unsigned retro_get_region(void) { return RETRO_REGION_NTSC; }
void *retro_get_memory_data(unsigned id) { (void)id; return NULL; }
size_t retro_get_memory_size(unsigned id) { (void)id; return 0; }
