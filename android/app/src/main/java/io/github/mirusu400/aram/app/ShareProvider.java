package io.github.mirusu400.aram.app;

import android.content.ContentProvider;
import android.content.ContentValues;
import android.content.Context;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.net.Uri;
import android.os.ParcelFileDescriptor;
import android.provider.OpenableColumns;

import java.io.File;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.util.Locale;

/**
 * Serves artifacts the frontend writes below app-private storage - save
 * backups above all - to whichever app the user picks from the share sheet.
 * Private storage is unreachable by any file manager, so without this a save
 * backup could be written but never retrieved.
 *
 * The provider is not exported: a receiver only ever reaches a file through
 * the read grant attached to the share intent, and only below the artifact
 * root.
 */
public final class ShareProvider extends ContentProvider {
    static final String AUTHORITY_SUFFIX = ".shares";

    private static final String[] DEFAULT_PROJECTION = {
            OpenableColumns.DISPLAY_NAME,
            OpenableColumns.SIZE,
    };

    /**
     * The frontend's artifact root. ConfigureStorage points XDG_CONFIG_HOME at
     * {@code filesDir/config}, and the shared Go layer writes every artifact
     * below {@code $XDG_CONFIG_HOME/ARAM}.
     */
    static File artifactRoot(Context context) throws IOException {
        return new File(new File(context.getFilesDir(), "config"), "ARAM")
                .getCanonicalFile();
    }

    /**
     * Builds the content URI that exposes {@code file}, which must be a
     * canonical path below the artifact root.
     */
    static Uri uriFor(Context context, File file) throws IOException {
        File root = artifactRoot(context);
        String relative = relativePath(root, file);
        if (relative == null) {
            throw new IOException("the file is outside the ARAM artifact folder");
        }
        Uri.Builder builder = new Uri.Builder()
                .scheme("content")
                .authority(context.getPackageName() + AUTHORITY_SUFFIX);
        for (String segment : relative.split("/")) {
            builder.appendPath(segment);
        }
        return builder.build();
    }

    private static String relativePath(File root, File file) {
        String rootPath = root.getPath() + File.separator;
        String filePath = file.getPath();
        if (!filePath.startsWith(rootPath)) {
            return null;
        }
        String relative = filePath.substring(rootPath.length());
        return relative.isEmpty() ? null : relative.replace(File.separatorChar, '/');
    }

    private File resolve(Uri uri) throws FileNotFoundException {
        Context context = getContext();
        if (context == null) {
            throw new FileNotFoundException("provider context is unavailable");
        }
        try {
            File root = artifactRoot(context);
            String path = uri.getPath();
            while (path != null && path.startsWith("/")) {
                path = path.substring(1);
            }
            if (path == null || path.isEmpty()) {
                throw new FileNotFoundException(uri.toString());
            }
            File file = new File(root, path).getCanonicalFile();
            if (relativePath(root, file) == null || !file.isFile()) {
                throw new FileNotFoundException(uri.toString());
            }
            return file;
        } catch (IOException error) {
            FileNotFoundException notFound = new FileNotFoundException(uri.toString());
            notFound.initCause(error);
            throw notFound;
        }
    }

    @Override
    public boolean onCreate() {
        return true;
    }

    @Override
    public Cursor query(
            Uri uri,
            String[] projection,
            String selection,
            String[] selectionArgs,
            String sortOrder
    ) {
        File file;
        try {
            file = resolve(uri);
        } catch (FileNotFoundException error) {
            return new MatrixCursor(DEFAULT_PROJECTION, 0);
        }
        if (projection == null) {
            projection = DEFAULT_PROJECTION;
        }
        MatrixCursor cursor = new MatrixCursor(projection, 1);
        Object[] row = new Object[projection.length];
        for (int index = 0; index < projection.length; index++) {
            if (OpenableColumns.DISPLAY_NAME.equals(projection[index])) {
                row[index] = file.getName();
            } else if (OpenableColumns.SIZE.equals(projection[index])) {
                row[index] = file.length();
            }
        }
        cursor.addRow(row);
        return cursor;
    }

    @Override
    public String getType(Uri uri) {
        String path = uri.getPath();
        if (path == null) {
            return "application/octet-stream";
        }
        String lower = path.toLowerCase(Locale.ROOT);
        if (lower.endsWith(".png")) {
            return "image/png";
        }
        if (lower.endsWith(".json")) {
            return "application/json";
        }
        if (lower.endsWith(".zip")) {
            return "application/zip";
        }
        if (lower.endsWith(".log") || lower.endsWith(".txt")) {
            return "text/plain";
        }
        // .aramsave and anything else: ARAM's own formats have no registered
        // type, and a generic one keeps every target app in the chooser.
        return "application/octet-stream";
    }

    @Override
    public ParcelFileDescriptor openFile(Uri uri, String mode) throws FileNotFoundException {
        if (!"r".equals(mode)) {
            throw new SecurityException("shared artifacts are read-only");
        }
        return ParcelFileDescriptor.open(resolve(uri), ParcelFileDescriptor.MODE_READ_ONLY);
    }

    @Override
    public Uri insert(Uri uri, ContentValues values) {
        throw new UnsupportedOperationException("shared artifacts are read-only");
    }

    @Override
    public int delete(Uri uri, String selection, String[] selectionArgs) {
        throw new UnsupportedOperationException("shared artifacts are read-only");
    }

    @Override
    public int update(
            Uri uri,
            ContentValues values,
            String selection,
            String[] selectionArgs
    ) {
        throw new UnsupportedOperationException("shared artifacts are read-only");
    }
}
