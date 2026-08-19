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

/**
 * Serves verified product packages from the app-private update folder to the
 * system package installer. The provider is not exported; the installer only
 * reaches a package through the read grant attached to the install intent.
 */
public final class UpdateProvider extends ContentProvider {
    static final String UPDATES_DIRECTORY = "updates";
    static final String PACKAGE_MIME_TYPE = "application/vnd.android.package-archive";

    private static final String AUTHORITY_SUFFIX = ".updates";
    private static final String[] DEFAULT_PROJECTION = {
            OpenableColumns.DISPLAY_NAME,
            OpenableColumns.SIZE,
    };

    static File updatesDirectory(Context context) throws IOException {
        return new File(context.getFilesDir(), UPDATES_DIRECTORY).getCanonicalFile();
    }

    /**
     * Builds the content URI that exposes {@code file}, which must already be
     * a canonical path below the update folder.
     */
    static Uri uriFor(Context context, File file) throws IOException {
        File updates = updatesDirectory(context);
        String relative = relativePath(updates, file);
        if (relative == null) {
            throw new IOException("the package is outside the update folder");
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
            File updates = updatesDirectory(context);
            String path = uri.getPath();
            while (path != null && path.startsWith("/")) {
                path = path.substring(1);
            }
            if (path == null || path.isEmpty()) {
                throw new FileNotFoundException(uri.toString());
            }
            File file = new File(updates, path).getCanonicalFile();
            if (relativePath(updates, file) == null || !file.isFile()) {
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
        return PACKAGE_MIME_TYPE;
    }

    @Override
    public ParcelFileDescriptor openFile(Uri uri, String mode) throws FileNotFoundException {
        if (!"r".equals(mode)) {
            throw new SecurityException("update packages are read-only");
        }
        return ParcelFileDescriptor.open(resolve(uri), ParcelFileDescriptor.MODE_READ_ONLY);
    }

    @Override
    public Uri insert(Uri uri, ContentValues values) {
        throw new UnsupportedOperationException("update packages are read-only");
    }

    @Override
    public int delete(Uri uri, String selection, String[] selectionArgs) {
        throw new UnsupportedOperationException("update packages are read-only");
    }

    @Override
    public int update(
            Uri uri,
            ContentValues values,
            String selection,
            String[] selectionArgs
    ) {
        throw new UnsupportedOperationException("update packages are read-only");
    }
}
